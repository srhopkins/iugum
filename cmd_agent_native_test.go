package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeWikiSearchRespectsPolicyAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.md")
	os.WriteFile(outside, []byte("needle private"), 0600)
	os.Symlink(outside, filepath.Join(root, "linked.md"))
	os.WriteFile(filepath.Join(root, "denied.md"), []byte("needle denied"), 0600)
	os.WriteFile(filepath.Join(root, "allowed.md"), []byte(strings.Repeat("opening ", 300)+"needle useful passage"), 0600)
	hits, e := searchAgentWiki(context.Background(), root, "needle", nil, func(_ context.Context, obj, act string) error {
		if obj == "wiki:denied.md" {
			return errors.New("denied")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(hits) != 1 || hits[0].Path != "allowed.md" {
		t.Fatalf("unexpected hits %#v", hits)
	}
	if !strings.Contains(hits[0].Text, "needle useful passage") {
		t.Fatal("snippet omitted match")
	}
	hits, e = searchAgentWiki(context.Background(), root, "needle", []string{"useful"}, func(context.Context, string, string) error { return errors.New("denied") })
	if e != nil || len(hits) != 0 {
		t.Fatal("policy did not deny all")
	}
}
func TestNativePolicyReloadsRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.csv")
	os.WriteFile(path, []byte("p, agent:chief, *, *, allow\n"), 0600)
	gate := agentFilePolicy{path: path}
	req := contract.Request{Sub: "agent:chief", Obj: "project:secret", Act: "read"}
	if e := gate.Enforce(context.Background(), req); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("p, agent:chief, *, *, allow\np, agent:chief, project:secret, read, deny\n"), 0600)
	if e := gate.Enforce(context.Background(), req); e == nil {
		t.Fatal("revocation not enforced")
	}
}

func TestNativeCommitmentToolsRequireCurrentAuthority(t *testing.T) {
	desk, e := agentdesk.New(agentdesk.Config{DataDir: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	tools := nativeCommitmentTools(desk, "I will ship this tomorrow")
	if _, e = tools[0].Execute(context.Background(), json.RawMessage(`{"title":"Ship work","user_quote":"someone else promised"}`)); e == nil {
		t.Fatal("accepted retrieved authority")
	}
	b, e := tools[0].Execute(context.Background(), json.RawMessage(`{"title":"Ship work","due":"2026-09-12","user_quote":"I will ship this tomorrow"}`))
	if e != nil {
		t.Fatal(e)
	}
	var c agentdesk.Commitment
	if e = json.Unmarshal([]byte(b), &c); e != nil {
		t.Fatal(e)
	}
	if c.Due != "2026-09-12" {
		t.Fatal(c)
	}
	tools = nativeCommitmentTools(desk, "Actually defer that")
	b, e = tools[1].Execute(context.Background(), json.RawMessage(`{"id":"`+c.ID+`","action":"defer","user_quote":"Actually defer that"}`))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(b), &c); e != nil || !c.Deferred {
		t.Fatal(c, e)
	}
}

type routingTestPolicy struct{}

func (routingTestPolicy) Enforce(context.Context, contract.Request) error { return nil }
func TestNativeRoutingSelectionPersistenceAndTransport(t *testing.T) {
	cfg := nativeAgentConfig{Models: agentcore.Config{DefaultProfile: "local", Pools: map[string]agentcore.Pool{"local": {BaseURL: "http://127.0.0.1:11434/v1"}}, Profiles: map[string]agentcore.Profile{"local": {Pool: "local", Model: "test", ZeroCost: true}}}}
	runtime, e := agentcore.New(cfg.Models, "", routingTestPolicy{})
	if e != nil {
		t.Fatal(e)
	}
	selected := ""
	p := filepath.Join(t.TempDir(), "routing-state.json")
	check := func(context.Context, string, string) error { return nil }
	tools := nativeRoutingTools(runtime, cfg, &selected, p, check)
	if _, e = tools[1].Execute(context.Background(), json.RawMessage(`{"profile":"local"}`)); e != nil {
		t.Fatal(e)
	}
	if selected != "local" {
		t.Fatal(selected)
	}
	if b, e := os.ReadFile(p); e != nil || !strings.Contains(string(b), "local") {
		t.Fatal(string(b), e)
	}
	cfg.ChatTransport = "codex-subscription"
	tools = nativeRoutingTools(runtime, cfg, &selected, p, check)
	if _, e = tools[1].Execute(context.Background(), json.RawMessage(`{"profile":"local"}`)); e == nil {
		t.Fatal("silently switched transport")
	}
	if text, e := tools[0].Execute(context.Background(), nil); e != nil || !strings.Contains(text, `"native_selection_available":false`) {
		t.Fatal(text, e)
	}
	cfg.ChatTransport = "native"
	tools = nativeRoutingTools(runtime, cfg, &selected, p, func(context.Context, string, string) error { return errors.New("denied") })
	if _, e = tools[1].Execute(context.Background(), json.RawMessage(`{"profile":"local"}`)); e == nil {
		t.Fatal("selection bypassed policy")
	}
}

func TestNativeAgentMemoryUsesOwnDirectory(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	parentDir := filepath.Join(root, "parent")
	cloneDir := filepath.Join(root, "clone")
	t.Setenv("IUGUM_DATA", parentDir)
	parent, e := openNativeAgentMemory(parentDir)
	if e != nil {
		t.Fatal(e)
	}
	defer parent.Close()
	clone, e := openNativeAgentMemory(cloneDir)
	if e != nil {
		t.Fatal(e)
	}
	defer clone.Close()
	if e = parent.Remember(ctx, contract.MemoryRec{NS: "same", Type: "fact", Key: "preference", Value: "parent"}); e != nil {
		t.Fatal(e)
	}
	if _, found, e := clone.Recall(ctx, "same", "preference"); e != nil || found {
		t.Fatal("parent leaked into clone", e)
	}
	if e = clone.Remember(ctx, contract.MemoryRec{NS: "same", Type: "fact", Key: "preference", Value: "clone"}); e != nil {
		t.Fatal(e)
	}
	rec, found, e := parent.Recall(ctx, "same", "preference")
	if e != nil || !found || rec.Value != "parent" {
		t.Fatal(rec, e)
	}
	reopened, e := openNativeAgentMemory(cloneDir)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	rec, found, e = reopened.Recall(ctx, "same", "preference")
	if e != nil || !found || rec.Value != "clone" {
		t.Fatal(rec, e)
	}
}

func TestSubscriptionProfileSelection(t *testing.T) {
	c := nativeAgentConfig{ChatTransport: "codex-subscription", SubscriptionModel: "default-model", SubscriptionProfiles: map[string]string{"fast": "small-model", "deep": "large-model"}, Models: agentcore.Config{DefaultProfile: "local", Pools: map[string]agentcore.Pool{"local": {BaseURL: "http://127.0.0.1:11434/v1"}}, Profiles: map[string]agentcore.Profile{"local": {Pool: "local", Model: "test"}}}}
	r, e := agentcore.New(c.Models, "", routingTestPolicy{})
	if e != nil {
		t.Fatal(e)
	}
	native, sub := "local", ""
	path := filepath.Join(t.TempDir(), "routing.json")
	var checked []string
	check := func(_ context.Context, obj, act string) error { checked = append(checked, obj+"/"+act); return nil }
	tools := nativeRoutingToolsWithSubscription(r, c, &native, &sub, path, check)
	if _, e = tools[1].Execute(context.Background(), json.RawMessage(`{"profile":"deep"}`)); e != nil {
		t.Fatal(e)
	}
	if sub != "deep" || native != "local" || selectedSubscriptionModel(c, sub) != "large-model" {
		t.Fatal(sub, native)
	}
	if !strings.Contains(strings.Join(checked, " "), "agent/model/codex-subscription/large-model/call") {
		t.Fatal(checked)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"subscription_profile":"deep"`) {
		t.Fatal(string(b))
	}
	status, e := tools[0].Execute(context.Background(), nil)
	if e != nil || !strings.Contains(status, `"subscription_model":"large-model"`) {
		t.Fatal(status, e)
	}
	if _, e = tools[1].Execute(context.Background(), json.RawMessage(`{"profile":"arbitrary"}`)); e == nil {
		t.Fatal("unconfigured model selected")
	}
	denied := nativeRoutingToolsWithSubscription(r, c, &native, &sub, path, func(context.Context, string, string) error { return errors.New("denied") })
	if _, e = denied[1].Execute(context.Background(), json.RawMessage(`{"profile":"fast"}`)); e == nil || sub != "deep" {
		t.Fatal("denial changed model")
	}
	if selectedSubscriptionModel(c, "") != "default-model" {
		t.Fatal("default lost")
	}
}
