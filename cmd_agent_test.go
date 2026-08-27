package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAgentScaffoldAndParse(t *testing.T) {
	cwd := t.TempDir()
	var warnings bytes.Buffer
	if err := initAgent(cwd, "scout", &warnings); err != nil {
		t.Fatal(err)
	}
	if warnings.Len() != 0 {
		t.Fatalf("outside git must not warn: %s", warnings.String())
	}

	root := filepath.Join(cwd, "scout")
	cfg, err := LoadAgentFile(filepath.Join(root, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "scout" || cfg.Image != defaultAgentImage {
		t.Fatalf("identity = %+v", cfg)
	}
	if cfg.Network.Name != "scout" || cfg.Network.Mode != "open" {
		t.Fatalf("network = %+v", cfg.Network)
	}
	if cfg.Startup.Restart != "unless-stopped" {
		t.Fatalf("startup = %+v", cfg.Startup)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("mounts = %+v", cfg.Mounts)
	}
	for _, path := range []string{
		filepath.Join(root, "home", "policy.csv"),
		filepath.Join(root, "home", ".iugum-probe"),
		filepath.Join(root, "data", ".iugum-probe"),
	} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("%s missing or empty: %v", path, err)
		}
	}
}

func TestInitAgentRefusesExistingConfig(t *testing.T) {
	cwd := t.TempDir()
	if err := initAgent(cwd, "worker", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, "worker", "agent.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := initAgent(cwd, "worker", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("second init must refuse overwrite: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("second init changed agent.yaml")
	}
}

func TestParseAgentFileOptionalSchema(t *testing.T) {
	cfg, err := ParseAgentFile([]byte(`name: locked
image: example:v1
mounts:
  - target: /run/secrets
    tmpfs: true
    ro: true
ports:
  - "127.0.0.1:8080:8080"
network:
  name: locked
  mode: locked
privileges:
  cap_add: [NET_ADMIN]
startup:
  restart: "no"
  env: [SSH_AUTH_SOCK]
jobs: jobs.yaml
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mounts[0].Tmpfs || !cfg.Mounts[0].RO {
		t.Fatalf("tmpfs mount = %+v", cfg.Mounts[0])
	}
	if cfg.Network.Mode != "locked" || cfg.Privileges.CapAdd[0] != "NET_ADMIN" {
		t.Fatalf("optional schema = %+v", cfg)
	}
	if cfg.Startup.Restart != "no" || cfg.Jobs != "jobs.yaml" {
		t.Fatalf("startup/jobs = %+v", cfg)
	}
}

func TestAgentGitIgnoreWarning(t *testing.T) {
	cwd := t.TempDir()
	if err := exec.Command("git", "init", "-q", cwd).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	var warnings bytes.Buffer
	if err := initAgent(cwd, "visible", &warnings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings.String(), "visible/home/") || !strings.Contains(warnings.String(), "visible/data/") {
		t.Fatalf("warning = %q", warnings.String())
	}

	ignored := t.TempDir()
	if err := exec.Command("git", "init", "-q", ignored).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, ".gitignore"), []byte("*/home/\n*/data/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings.Reset()
	if err := initAgent(ignored, "private", &warnings); err != nil {
		t.Fatal(err)
	}
	if warnings.Len() != 0 {
		t.Fatalf("ignored paths must not warn: %s", warnings.String())
	}
}

func TestValidAgentName(t *testing.T) {
	for _, good := range []string{"worker", "worker-1", "worker_one"} {
		if !validAgentName(good) {
			t.Errorf("%q must be valid", good)
		}
	}
	for _, bad := range []string{"", ".", "..", "../escape", "nested/name", "/absolute"} {
		if validAgentName(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}
