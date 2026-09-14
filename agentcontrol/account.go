package agentcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"os/exec"
	"strings"
	"time"
)

func claudeEnvironment(configDir string) []string {
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "HOME", "PATH", "USER", "LOGNAME", "TMPDIR", "LANG", "LC_ALL", "SHELL":
			env = append(env, entry)
		}
	}
	return append(env, "CLAUDE_CONFIG_DIR="+configDir)
}

// verifyClaudeAccount fails closed when account identity cannot be established.
// Account must be the actual subscription login email, not a friendly alias.
func verifyClaudeAccount(ctx context.Context, binary string, r StartRequest) error {
	expected := strings.TrimSpace(r.Account)
	address, err := mail.ParseAddress(expected)
	if err != nil || !strings.EqualFold(address.Address, expected) {
		return errors.New("agentcontrol: account must be the expected Claude login email")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, binary, "auth", "status", "--json")
	cmd.Dir = r.Project
	cmd.Env = claudeEnvironment(r.ConfigDir)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("agentcontrol: cannot verify Claude login for %s; run Claude auth status with the configured directory", expected)
	}
	var status struct {
		LoggedIn    bool   `json:"loggedIn"`
		Email       string `json:"email"`
		AuthMethod  string `json:"authMethod"`
		APIProvider string `json:"apiProvider"`
	}
	if err = json.Unmarshal(out, &status); err != nil {
		return errors.New("agentcontrol: invalid Claude authentication status")
	}
	if !status.LoggedIn || !strings.EqualFold(strings.TrimSpace(status.Email), expected) {
		return fmt.Errorf("agentcontrol: Claude login does not match expected account %s; delegation refused", expected)
	}
	if status.AuthMethod != "claude.ai" && status.AuthMethod != "oauth_token" {
		return fmt.Errorf("agentcontrol: subscription authentication method %q is not verified; delegation refused", status.AuthMethod)
	}
	if status.APIProvider != "firstParty" {
		return errors.New("agentcontrol: Claude is configured for another API provider; delegation refused")
	}
	return nil
}
