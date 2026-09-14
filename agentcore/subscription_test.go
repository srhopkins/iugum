package agentcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeCLI(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestSubscriptionUsage(t *testing.T) {
	binary := fakeCLI(t, `
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-o" ]; then shift; output="$1"; fi
 shift
done
prompt=$(cat)
case "$prompt" in *"Current timestamp:"*) ;; *) exit 9;; esac
printf 'A concise answer' > "$output"
printf '%s\n' '{"type":"thread.started"}' '{"type":"item.completed","item":{"type":"error","message":"Code Mode unavailable: disabled"}}' '{"type":"item.completed","item":{"type":"agent_message"}}' '{"type":"turn.completed","usage":{"input_tokens":123,"output_tokens":7}}'
`)
	r, _ := New(config("http://localhost:1"), "", gate{})
	out, err := r.RunSubscription(context.Background(), RunRequest{Actor: "chief", Messages: []Message{{Role: "user", Content: "status"}}}, binary, "test")
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "A concise answer" || len(out.Warnings) != 3 {
		t.Fatal(out)
	}
	u := r.Usage()[0]
	if !u.UsageKnown || u.PriceKnown || u.InputTokens != 123 || u.OutputTokens != 7 || u.Pool != "codex-subscription" {
		t.Fatal(u)
	}
}
func TestSubscriptionRejectsTools(t *testing.T) {
	binary := fakeCLI(t, `printf '%s\n' '{"type":"item.started","item":{"type":"command_execution"}}'`)
	r, _ := New(config("http://localhost:1"), "", gate{})
	_, err := r.RunSubscription(context.Background(), RunRequest{Actor: "chief"}, binary, "test")
	if err == nil || !strings.Contains(err.Error(), "disallowed item") {
		t.Fatal(err)
	}
}
func TestSubscriptionPolicy(t *testing.T) {
	r, _ := New(config("http://localhost:1"), "", gate{deny: "codex-subscription"})
	_, err := r.RunSubscription(context.Background(), RunRequest{Actor: "chief"}, "/does/not/exist", "test")
	if err == nil || !strings.Contains(err.Error(), "policy denied") {
		t.Fatal(err)
	}
}
