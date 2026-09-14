package agentsessions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var ErrLiveSteeringUnsupported = errors.New("steering an unmanaged Claude process is unavailable; use agentcontrol for managed streaming sessions or resume a saved session")

type Claude struct{ Executable string }
type ResumeRequest struct {
	SessionID, Project, Account, ConfigDir, Prompt string
	AllowedTools                                   []string
}
type ResumeResult struct {
	Output             string
	Account, SessionID string
	StartedAt          time.Time
}

// Resume requires an explicit account label and config directory: ambient credentials are not account proof.
// The caller must authorize this action through its policy gate before invoking it.
// Claude retains its native permission checks; this adapter never enables bypass.
func (c Claude) Resume(ctx context.Context, r ResumeRequest) (ResumeResult, error) {
	out := ResumeResult{Account: r.Account, SessionID: r.SessionID, StartedAt: time.Now().UTC()}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`).MatchString(r.SessionID) {
		return out, errors.New("invalid session ID")
	}
	if r.Project == "" || r.Account == "" || r.ConfigDir == "" || strings.TrimSpace(r.Prompt) == "" {
		return out, errors.New("session project, account label, config directory, and prompt are required")
	}
	for _, p := range []string{r.Project, r.ConfigDir} {
		st, e := os.Stat(p)
		if e != nil || !st.IsDir() {
			return out, fmt.Errorf("directory unavailable: %s", p)
		}
	}
	executable := c.Executable
	if executable == "" {
		executable = "claude"
	}
	args := []string{"--resume", r.SessionID, "--print", "--output-format", "json"}
	if len(r.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(r.AllowedTools, ","))
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = r.Project
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "CLAUDE_CONFIG_DIR=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "CLAUDE_CONFIG_DIR="+r.ConfigDir)
	cmd.Stdin = strings.NewReader("Current time: " + out.StartedAt.Format(time.RFC3339) + ". Use dated evidence for historical claims.\n\n" + r.Prompt)
	b, e := cmd.Output()
	out.Output = string(b)
	return out, e
}
func (c Claude) Steer(context.Context, ResumeRequest) error { return ErrLiveSteeringUnsupported }
