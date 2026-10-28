package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/beadactivity"
)

func runBeadActivity(ctx context.Context, a *app.App, args []string) int {
	fs := flag.NewFlagSet("bead-activity", flag.ContinueOnError)
	actor := fs.String("actor", os.Getenv("BEADS_ACTOR"), "Current ticket assignee (required)")
	dir := fs.String("dir", "", "Tracker directory; default current directory")
	reason := fs.String("reason", "", "Short pause/completion reason; never include secrets")
	ttl := fs.Duration("ttl", 2*time.Minute, "Working expiry, at most 10m; repeat before expiry while working")
	runID := fs.String("run", "", "Explicit execution id; never inferred from assignee")
	platform := fs.String("platform", "", "Agent platform, e.g. codex or claude")
	session := fs.String("session", "", "Verified platform session/thread identifier")
	contextSession := fs.String("context-session", "", "Separate context session identifier, only if verified")
	agent := fs.String("agent-id", "", "Verified agent id, distinct from resumable session")
	parentSession := fs.String("parent-session", "", "Verified parent session id")
	parentAgent := fs.String("parent-agent", "", "Verified parent agent id")
	container := fs.String("container", "", "Container name when applicable")
	transcript := fs.String("transcript", "", "Verified transcript locator; text only, no browser file access")
	threadURL := fs.String("thread-url", "", "Verified thread link")
	unavailable := fs.String("identity-unavailable", "", "Why actual platform session/agent identity is unavailable")
	label := fs.String("label", "", "Human-readable worker label")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "Usage: iugum bead-activity [--actor OWNER] [--dir DIR] [--ttl 2m] [--reason TEXT] <issue> <working|paused|completed>")
		return 2
	}
	if err := a.Check(ctx, "tracker", "run"); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	run := func(argv ...string) ([]byte, error) {
		base := []string{"beads"}
		if *dir != "" {
			base = append(base, "-C", *dir)
		}
		cmd := exec.CommandContext(ctx, exe, append(base, argv...)...)
		cmd.Stderr = io.Discard // provider/config errors must not leak credentials into activity output
		out, e := cmd.Output()
		if e != nil {
			return nil, fmt.Errorf("canonical Beads command failed: %w", e)
		}
		return out, nil
	}
	host, _ := os.Hostname()
	cwd, _ := os.Getwd()
	ref := beadactivity.Execution{RunID: *runID, Actor: *actor, Label: *label, Platform: *platform, Host: host, Container: *container, SessionID: *session, ContextSessionID: *contextSession, AgentID: *agent, ParentSessionID: *parentSession, ParentAgentID: *parentAgent, Transcript: *transcript, ThreadURL: *threadURL, IdentityUnavailable: *unavailable, CWD: cwd}
	if err := beadactivity.ReportExecution(run, ref, fs.Arg(0), fs.Arg(1), *reason, time.Now(), *ttl); err != nil {
		fmt.Fprintln(os.Stderr, "bead-activity:", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "Activity recorded; ticket status and owner unchanged.")
	return 0
}
