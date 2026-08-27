package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/srhopkins/iugum/app"
	"gopkg.in/yaml.v3"
)

const agentUsage = `Usage: iugum agent init <name>

  init <name>    create agent.yaml, home/, data/, probes, and starter policy
`

const starterAgentPolicy = `# Casbin policy: subject, object, action, effect
# Replace this row with narrower rules when the agent needs restrictions.
p, *, *, *, allow
`

func runAgent(ctx context.Context, a *app.App, args []string) int {
	return runAgentIO(ctx, a, args, os.Stdout, os.Stderr)
}

func runAgentIO(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, agentUsage)
		return 0
	}
	if args[0] != "init" {
		fmt.Fprintf(stderr, "agent: unknown subcommand %s\n\n%s", args[0], agentUsage)
		return 2
	}
	if len(args) != 2 || !validAgentName(args[1]) {
		fmt.Fprintln(stderr, "Usage: iugum agent init <name>")
		fmt.Fprintln(stderr, "agent init: name must be one directory name")
		return 2
	}
	if err := a.Check(ctx, "agent", "init"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "agent init: %v\n", err)
		return 1
	}
	if err := initAgent(cwd, args[1], stderr); err != nil {
		fmt.Fprintf(stderr, "agent init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "iugum agent: created %s/agent.yaml, home/, and data/\n", args[1])
	return 0
}

func validAgentName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!filepath.IsAbs(name) && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\`)
}

func initAgent(cwd, name string, warnings io.Writer) error {
	root := filepath.Join(cwd, name)
	configPath := filepath.Join(root, "agent.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return errors.New("agent.yaml already exists; refusing to overwrite it")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	home := filepath.Join(root, "home")
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}

	cfg := AgentFile{
		Name:  name,
		Image: defaultAgentImage,
		Mounts: []AgentMount{
			{Source: "./home", Target: "/home/iugum"},
			{Source: "./data", Target: "/data"},
		},
		Network: AgentNetwork{Name: name, Mode: defaultNetworkMode},
		Startup: AgentStartup{Restart: defaultRestart},
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("agent.yaml already exists; refusing to overwrite it")
		}
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	warnIfAgentDirsTracked(cwd, name, warnings)

	files := map[string]string{
		filepath.Join(home, "policy.csv"):   starterAgentPolicy,
		filepath.Join(home, ".iugum-probe"): "iugum agent home\n",
		filepath.Join(data, ".iugum-probe"): "iugum agent data\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func warnIfAgentDirsTracked(cwd, name string, warnings io.Writer) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = cwd
	if err := cmd.Run(); err != nil {
		return
	}
	var unignored []string
	for _, dir := range []string{"home", "data"} {
		rel := filepath.Join(name, dir) + string(os.PathSeparator)
		check := exec.Command("git", "check-ignore", "-q", "--", rel)
		check.Dir = cwd
		if err := check.Run(); err != nil {
			unignored = append(unignored, rel)
		}
	}
	if len(unignored) > 0 {
		fmt.Fprintf(warnings, "warning: agent state is not gitignored: %s; add ignore rules before storing private data\n", strings.Join(unignored, ", "))
	}
}
