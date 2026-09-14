package agentcontrol

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyClaudeAccount(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantError      bool
	}{{"matching", `{"loggedIn":true,"email":"chief@example.org","authMethod":"claude.ai","apiProvider":"firstParty"}`, false}, {"different", `{"loggedIn":true,"email":"contract@example.org","authMethod":"claude.ai","apiProvider":"firstParty"}`, true}, {"api-key", `{"loggedIn":true,"email":"chief@example.org","authMethod":"api_key","apiProvider":"firstParty"}`, true}, {"logged-out", `{"loggedIn":false}`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "claude")
			body := "#!/bin/sh\n[ \"$1 $2 $3\" = \"auth status --json\" ] || exit 5\ncat <<'STATUS'\n" + tc.response + "\nSTATUS\n"
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			err := verifyClaudeAccount(context.Background(), bin, StartRequest{Account: "chief@example.org", ConfigDir: dir, Project: dir})
			if (err != nil) != tc.wantError {
				t.Fatal(err)
			}
		})
	}
}
func TestStartIdentityFailureNeverSpawns(t *testing.T) {
	m, r := setup(t)
	m.VerifyAccount = func(context.Context, string, StartRequest) error { return os.ErrPermission }
	_, err := m.Start(context.Background(), r)
	if err == nil {
		t.Fatal("unverified launch")
	}
	states, _ := m.List(context.Background(), r.Actor)
	if len(states) != 0 {
		t.Fatal(states)
	}
}
func TestClaudeEnvironmentDropsProviderOverrides(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "wrong-account")
	t.Setenv("ANTHROPIC_BASE_URL", "https://example.invalid")
	env := strings.Join(claudeEnvironment("/tmp/account"), "\n")
	for _, name := range []string{"ANTHROPIC_API_KEY=", "CLAUDE_CODE_OAUTH_TOKEN=", "ANTHROPIC_BASE_URL="} {
		if strings.Contains(env, name) {
			t.Fatal("inherited override", name)
		}
	}
}
