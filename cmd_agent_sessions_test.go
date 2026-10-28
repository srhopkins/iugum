package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytes"
	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/agentplatform"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
)

func TestAgentSessionsConfigDefaults(t *testing.T) {
	ps, err := sessionProviders(sessionProviderConfig{DataDir: t.TempDir()}, nil)
	if err != nil || len(ps) != 2 {
		t.Fatalf("providers=%v err=%v", ps, err)
	}
	for i, want := range []string{"Codex", "Cursor"} {
		if ps[i].Name() != want {
			t.Fatal(ps[i].Name())
		}
	}
}
func TestAgentSessionsConfigPartialFailsClosed(t *testing.T) {
	for _, c := range []sessionProviderConfig{{ClaudeAccount: "a@example.com"}, {ClaudeActor: "steve"}, {OpenCodeURL: "http://127.0.0.1:1234"}, {OpenCodePassword: "secret"}, {OpenCodeURL: "http://example.com", OpenCodePassword: "secret"}, {OpenCodeURL: "http://u:secret@localhost", OpenCodePassword: "secret"}} {
		if _, e := sessionProviders(c, nil); e == nil || strings.Contains(e.Error(), "secret") {
			t.Fatalf("unsafe error %v", e)
		}
	}
}
func TestAgentSessionsConfigVerifiedAdapters(t *testing.T) {
	root := t.TempDir()
	pol := filepath.Join(root, "policy.csv")
	os.WriteFile(pol, []byte("p, steve, agent/session/*, start, deny\np, steve, agent/session/*, approve, deny\n"), 0600)
	c := sessionProviderConfig{DataDir: root, ClaudeBinary: "custom-claude", ClaudeActor: "steve", ClaudeAccount: "a@example.com", ClaudeConfigDir: root, ClaudeModel: "test-model", ClaudePolicy: pol, OpenCodeURL: "http://127.0.0.1:1234", OpenCodePassword: "secret"}
	host, e := policy.New("", pol)
	if e != nil {
		t.Fatal(e)
	}
	ps, e := sessionProviders(c, host)
	if e != nil {
		t.Fatal(e)
	}
	var cl agentplatform.ClaudeProvider
	var oc agentplatform.OpenCodeHTTPProvider
	for _, p := range ps {
		switch p := p.(type) {
		case agentplatform.ClaudeProvider:
			cl = p
		case agentplatform.OpenCodeHTTPProvider:
			oc = p
		}
	}
	if cl.Manager == nil || cl.Manager.VerifyAccount != nil || cl.Manager.Executable != "custom-claude" || cl.ConfigDir != root || cl.Model != "test-model" || oc.Password != "secret" {
		t.Fatal("incorrect adapter wiring")
	}
	// A denied policy must reach the manager, before any subprocess/account access.
	_, e = cl.Manager.Start(context.Background(), structStart(cl.Actor))
	if e == nil || !strings.Contains(e.Error(), "denied") {
		t.Fatalf("policy did not deny: %v", e)
	}
}
func structStart(actor string) agentcontrol.StartRequest {
	return agentcontrol.StartRequest{Actor: actor, ID: "test", Account: "a@example.com"}
}

func TestAgentSessionsConfigPolicyRouting(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "policy.csv")
	os.WriteFile(file, []byte("p, steve, *, *, allow\np, steve, agent/session/*, approve, deny\np, steve, agent/sessions, serve, deny\n"), 0600)
	host, e := policy.New("", file)
	if e != nil {
		t.Fatal(e)
	}
	gate := sessionPolicy{host: host, claude: policy.Live{Path: file}}
	if e = gate.Enforce(context.Background(), contract.Request{Sub: "steve", Obj: "agent/session/test", Act: "approve"}); e == nil {
		t.Fatal("approve bypassed host policy")
	}
	var out, errs bytes.Buffer
	t.Setenv("IUGUM_OPENCODE_URL", "")
	t.Setenv("OPENCODE_SERVER_PASSWORD", "")
	code := runAgentSessions(context.Background(), &app.App{Gate: host, Actor: "steve"}, []string{"serve", "--listen", "127.0.0.1:0", "--data-dir", root}, &out, &errs)
	if code != 1 || !strings.Contains(errs.String(), "denied") {
		t.Fatalf("code=%d err=%s", code, errs.String())
	}
}
func TestAgentSessionsConfigPrivateCredentialFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "password")
	os.WriteFile(file, []byte("private-password\n"), 0600)
	c := sessionProviderConfig{OpenCodeURL: "http://127.0.0.1:1234", OpenCodePasswordFile: file}
	ps, e := sessionProviders(c, nil)
	if e != nil {
		t.Fatal(e)
	}
	p := ps[2].(agentplatform.OpenCodeHTTPProvider)
	if p.Password != "private-password" {
		t.Fatal("file not loaded")
	}
	os.Chmod(file, 0644)
	if _, e = sessionProviders(c, nil); e == nil || strings.Contains(e.Error(), "private-password") {
		t.Fatalf("unsafe error %v", e)
	}
}
func TestAgentSessionsConfigFlagsPrivacy(t *testing.T) {
	host, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("OPENCODE_SERVER_PASSWORD", "private-password")
	t.Setenv("IUGUM_OPENCODE_URL", "http://user:private-password@127.0.0.1:1234")
	var out, errs bytes.Buffer
	code := runAgentSessions(context.Background(), &app.App{Gate: host}, []string{"serve", "--listen", "127.0.0.1:0"}, &out, &errs)
	if code != 2 || strings.Contains(out.String()+errs.String(), "private-password") {
		t.Fatalf("code=%d unsafe config diagnostic", code)
	}
	errs.Reset()
	code = runAgentSessions(context.Background(), &app.App{Gate: host}, []string{"serve", "--help"}, &out, &errs)
	if strings.Contains(out.String()+errs.String(), "private-password") {
		t.Fatal("help leaked environment URL credential")
	}
	errs.Reset()
	code = runAgentSessions(context.Background(), &app.App{Gate: host}, []string{"serve", "--claude-acp-bin", "prototype"}, &out, &errs)
	if code != 2 {
		t.Fatal("prototype fallback flag remains")
	}
}
