package main

import (
	"context"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCloneIsolation(t *testing.T) {
	d := t.TempDir()
	parent := filepath.Join(d, "parent")
	os.Mkdir(parent, 0700)
	os.WriteFile(filepath.Join(parent, "agent.yaml"), []byte("name: chief\nruntime: native\npolicy_file: policy.csv\ninstructions_file: soul.md\nwiki_dir: wiki\nwiki_port: 3001\ncustom_future:\n  keep: yes\nmodels:\n  pools:\n    openai:\n      api_key_file: key\n"), 0600)
	os.WriteFile(filepath.Join(parent, "policy.csv"), []byte("p, agent:chief, *, *, allow\np, agent:chief-other, *, *, deny\n"), 0600)
	os.WriteFile(filepath.Join(parent, "soul.md"), []byte("Be concise"), 0600)
	os.WriteFile(filepath.Join(parent, "key"), []byte("secret"), 0600)
	dest := filepath.Join(d, "clone")
	p, e := createNativeAgentClone(context.Background(), filepath.Join(parent, "agent.yaml"), "chief-next", dest)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	var c map[string]any
	yaml.Unmarshal(b, &c)
	if c["wiki_port"] != nil || c["wiki_url"] != "http://127.0.0.1:3001" {
		t.Fatal("clone must reuse parent wiki", c)
	}
	if c["name"] != "chief-next" || c["data_dir"] != "data" || c["custom_future"] == nil {
		t.Fatal(c)
	}
	if !strings.Contains(string(b), filepath.Join(parent, "key")) {
		t.Fatal("secret reference not resolved")
	}
	if _, e = os.Stat(filepath.Join(dest, "key")); !os.IsNotExist(e) {
		t.Fatal("secret copied")
	}
	policy, _ := os.ReadFile(filepath.Join(dest, "policy.csv"))
	if !strings.Contains(string(policy), "agent:chief-next,") || !strings.Contains(string(policy), "agent:chief-other,") {
		t.Fatal(string(policy))
	}
	if _, e = createNativeAgentClone(context.Background(), filepath.Join(parent, "agent.yaml"), "again", dest); e == nil {
		t.Fatal("overwrote existing clone")
	}
}
func TestClonePolicyTokens(t *testing.T) {
	p := "# agent:chief\np, agent:chief, project:agent:chief, read, allow\np, agent:chief-other, *, *, allow\ng, agent:chief, role:reader\n"
	got := clonePolicyActor(p, "agent:chief", "agent:next")
	if !strings.Contains(got, "p, agent:next, project:agent:chief,") || !strings.Contains(got, "# agent:chief") || !strings.Contains(got, "g, agent:next, role:reader") {
		t.Fatal(got)
	}
}

func TestCloneNameCannotInjectPolicy(t *testing.T) {
	for _, name := range []string{"chief, *, *, allow", "chief\np, *", "../chief", ""} {
		if validNativeCloneName(name) {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
}
