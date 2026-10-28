package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestAgentModelSetupPersistenceAndPrivacy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "agent.yaml")
	original := "name: test\npolicy_file: policy.csv\nmodels:\n  default_profile: mini\n  pools:\n    local:\n      base_url: http://localhost:4000/v1\n      api_key_env: PRIVATE_TOKEN\n  profiles:\n    mini:\n      pool: local\n      model: old\n"
	os.WriteFile(p, []byte(original), 0600)
	allow := func(context.Context, string, string) error { return nil }
	s, e := nativeModelSetup(context.Background(), p, allow, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !s.Pools["local"].CredentialConfigured {
		t.Fatal("credential status missing")
	}
	s.Profiles["mini"] = agentdesk.SetupProfile{Pool: "local", Model: "new"}
	pool := s.Pools["local"]
	pool.DiscoverModels = true
	s.Pools["local"] = pool
	s, e = nativeModelSetup(context.Background(), p, allow, &s.ModelSetup)
	if e != nil || !s.RestartRequired {
		t.Fatalf("%+v %v", s, e)
	}
	b, _ := os.ReadFile(p)
	reloaded, reloadErr := nativeModelSetup(context.Background(), p, allow, nil)
	if reloadErr != nil || !reloaded.Pools["local"].DiscoverModels {
		t.Fatalf("discovery setting did not round trip: %+v %v", reloaded, reloadErr)
	}
	if !strings.Contains(string(b), "PRIVATE_TOKEN") || !strings.Contains(string(b), "policy_file: policy.csv") {
		t.Fatal("unrelated fields lost")
	}
	before := string(b)
	deny := func(context.Context, string, string) error { return errors.New("denied") }
	if _, e = nativeModelSetup(context.Background(), p, deny, &s.ModelSetup); e == nil {
		t.Fatal("denied write accepted")
	}
	if _, e = nativeModelSetup(context.Background(), p, deny, nil); e == nil {
		t.Fatal("denied read accepted")
	}
	s.Pools["local"] = agentdesk.SetupPool{BaseURL: "http://user:secret@localhost"}
	if _, e = nativeModelSetup(context.Background(), p, allow, &s.ModelSetup); e == nil {
		t.Fatal("credential URL accepted")
	}
	b, _ = os.ReadFile(p)
	if string(b) != before {
		t.Fatal("failed save changed file")
	}
}

func TestAgentModelSetupCasbinAndAtomicFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	os.WriteFile(path, []byte("name: test\nmodels:\n  default_profile: mini\n  pools:\n    proxy:\n      base_url: http://localhost:4000/v1\n      api_key_file: private/key\n  profiles:\n    mini:\n      pool: proxy\n      model: old\n"), 0600)
	policyPath := filepath.Join(t.TempDir(), "policy.csv")
	os.WriteFile(policyPath, []byte("p, *, *, *, allow\np, *, agent/model-setup, write, deny\n"), 0600)
	check := func(ctx context.Context, obj, act string) error {
		gate, e := policy.New("", policyPath)
		if e != nil {
			return e
		}
		return gate.Enforce(ctx, contract.Request{Sub: "agent:test", Obj: obj, Act: act})
	}
	state, e := nativeModelSetup(context.Background(), path, check, nil)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), "private/key") {
		t.Fatal("credential reference leaked")
	}
	before, _ := os.ReadFile(path)
	if _, e = nativeModelSetup(context.Background(), path, check, &state.ModelSetup); e == nil {
		t.Fatal("Casbin deny ignored")
	}
	os.WriteFile(policyPath, []byte("p, *, *, *, allow\np, *, agent/model/proxy/mini, call, deny\n"), 0600)
	if _, e = nativeModelSetup(context.Background(), path, check, &state.ModelSetup); e == nil {
		t.Fatal("model deny ignored")
	}
	os.WriteFile(policyPath, []byte("p, *, *, *, allow\n"), 0600)
	os.Chmod(dir, 0500)
	defer os.Chmod(dir, 0700)
	if _, e = nativeModelSetup(context.Background(), path, check, &state.ModelSetup); e == nil {
		t.Fatal("unwritable directory accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failure damaged existing YAML")
	}
}

func TestAgentModelSetupRuntimeRestartAndBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	os.WriteFile(path, []byte("name: test\nmodels:\n  future_key: preserve-me\n  default_profile: mini\n  pools:\n    proxy:\n      monthly_target_usd: 20\n      base_url: http://localhost:4000/v1\n  profiles:\n    mini:\n      pool: proxy\n      model: old\n      zero_cost: true\n"), 0600)
	var mu sync.Mutex
	control := nativeModelSetupControl(path, func(context.Context, string, string) error { return nil }, &mu)
	state, e := control(context.Background(), nil)
	if e != nil || state.RestartRequired {
		t.Fatalf("initial %+v %v", state, e)
	}
	mu.Lock()
	if _, e = control(context.Background(), &state.ModelSetup); e == nil {
		t.Fatal("busy save accepted")
	}
	mu.Unlock()
	state.Profiles["mini"] = agentdesk.SetupProfile{Pool: "proxy", Model: "next"}
	saved, e := control(context.Background(), &state.ModelSetup)
	if e != nil || !saved.RestartRequired {
		t.Fatalf("save %+v %v", saved, e)
	}
	read, e := control(context.Background(), nil)
	if e != nil || !read.RestartRequired {
		t.Fatalf("pending state lost: %+v %v", read, e)
	}
	raw, _ := os.ReadFile(path)
	for _, field := range []string{"future_key: preserve-me", "monthly_target_usd: 20", "zero_cost: true"} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("lost %s", field)
		}
	}
}

func TestAgentModelSetupYAMLAliasesPreserveUnrelatedValues(t *testing.T) {
	for _, raw := range []string{
		"name: test\ndefaults: &base\n  default_profile: mini\n  pools:\n    proxy:\n      base_url: http://localhost:4000/v1\n      api_key_env: PRIVATE_TOKEN\n  profiles:\n    mini:\n      pool: proxy\n      model: old\nmodels:\n  <<: *base\n",
		"name: test\ndefaults: &base\n  default_profile: mini\n  pools:\n    proxy:\n      base_url: http://localhost:4000/v1\n      api_key_env: PRIVATE_TOKEN\n  profiles:\n    mini:\n      pool: proxy\n      model: old\nmodels: *base\n",
		"name: test\nmodels: &base\n  default_profile: mini\n  pools:\n    proxy:\n      base_url: http://localhost:4000/v1\n      api_key_env: PRIVATE_TOKEN\n  profiles:\n    mini:\n      pool: proxy\n      model: old\ndefaults: *base\n",
		"name: test\npool_defaults: &pool\n  base_url: http://localhost:4000/v1\n  api_key_env: PRIVATE_TOKEN\nmodels:\n  default_profile: mini\n  pools:\n    proxy: *pool\n  profiles:\n    mini:\n      pool: proxy\n      model: old\n",
	} {
		path := filepath.Join(t.TempDir(), "agent.yaml")
		os.WriteFile(path, []byte(raw), 0600)
		var before map[string]any
		yaml.Unmarshal([]byte(raw), &before)
		allow := func(context.Context, string, string) error { return nil }
		state, e := nativeModelSetup(context.Background(), path, allow, nil)
		if e != nil {
			t.Fatal(e)
		}
		state.Pools["proxy"] = agentdesk.SetupPool{BaseURL: "http://localhost:5000/v1"}
		if _, e = nativeModelSetup(context.Background(), path, allow, &state.ModelSetup); e != nil {
			t.Fatal(e)
		}
		saved, _ := os.ReadFile(path)
		var after map[string]any
		if e = yaml.Unmarshal(saved, &after); e != nil {
			t.Fatal(e)
		}
		delete(before, "models")
		models := after["models"]
		delete(after, "models")
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("unrelated YAML changed: before %+v after %+v", before, after)
		}
		if !strings.Contains(fmt.Sprint(models), "PRIVATE_TOKEN") {
			t.Fatal("inherited private credential reference lost")
		}
		if !strings.Contains(fmt.Sprint(models), "localhost:5000") {
			t.Fatal("model endpoint unchanged")
		}
	}
}
