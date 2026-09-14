package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/srhopkins/iugum/agentsessions"
	"github.com/srhopkins/iugum/app"
)

type claudeSessionResumer interface {
	Resume(context.Context, agentsessions.ResumeRequest) (agentsessions.ResumeResult, error)
}

func runAgentSession(ctx context.Context, a *app.App, args []string, out, errout io.Writer) int {
	return runAgentSessionWith(ctx, a, args, out, errout, agentsessions.Claude{})
}
func runAgentSessionWith(ctx context.Context, a *app.App, args []string, out, errout io.Writer, claude claudeSessionResumer) int {
	if len(args) == 0 {
		fmt.Fprintln(errout, "Usage: iugum agent session capabilities | resume --session ID --project PATH --account LABEL --config-dir DIR --prompt TEXT")
		return 2
	}
	if args[0] == "capabilities" {
		if len(args) != 1 {
			return 2
		}
		if err := a.Check(ctx, "agent:session", "capabilities"); err != nil {
			fmt.Fprintln(errout, app.DenyMessage(err))
			return 1
		}
		json.NewEncoder(out).Encode(map[string]any{"claude": map[string]any{"saved_session_resume": true, "running_session_steer": false, "reason": "Local Claude CLI supports --resume with --print; live injection was not verified. Background resume may fork a running session.", "timestamp": "Fresh timestamp at dispatch; external CLI internal calls are outside iugum control."}, "cursor": map[string]any{"implemented": false}, "opencode": map[string]any{"implemented": false}, "codex": map[string]any{"implemented": false}})
		return 0
	}
	if args[0] != "resume" {
		fmt.Fprintln(errout, "agent session: expected capabilities or resume; live steering is not implemented")
		return 2
	}
	fs := flag.NewFlagSet("agent session resume", flag.ContinueOnError)
	fs.SetOutput(errout)
	var r agentsessions.ResumeRequest
	fs.StringVar(&r.SessionID, "session", "", "saved Claude session ID")
	fs.StringVar(&r.Project, "project", "", "trusted project directory")
	fs.StringVar(&r.Account, "account", "", "explicit account label")
	fs.StringVar(&r.ConfigDir, "config-dir", "", "Claude account configuration directory")
	fs.StringVar(&r.Prompt, "prompt", "", "authorized instruction")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 || r.SessionID == "" || r.Project == "" || r.Account == "" || r.ConfigDir == "" || r.Prompt == "" {
		fmt.Fprintln(errout, "agent session resume: --session, --project, --account, --config-dir, and --prompt are required")
		return 2
	}
	var err error
	r.Project, err = filepath.Abs(r.Project)
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	r.ConfigDir, err = filepath.Abs(r.ConfigDir)
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	// Authorize canonical directories so a symlink cannot disguise a project scope.
	r.Project, err = filepath.EvalSymlinks(r.Project)
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	r.ConfigDir, err = filepath.EvalSymlinks(r.ConfigDir)
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	for _, check := range [][2]string{{"transcript:claude:" + r.Account, "resume"}, {"project:" + r.Project, "execute"}, {"credentials:claude:" + r.ConfigDir, "use"}} {
		if err = a.Check(ctx, check[0], check[1]); err != nil {
			fmt.Fprintln(errout, app.DenyMessage(err))
			return 1
		}
	}
	result, err := claude.Resume(ctx, r)
	if err != nil {
		fmt.Fprintln(errout, "agent session resume:", err)
		return 1
	}
	if err = json.NewEncoder(out).Encode(result); err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	return 0
}
