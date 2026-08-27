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

const agentUsage = `Usage: iugum agent <init|up|down|status|ls>

  init <name>    create agent.yaml, home/, data/, probes, and starter policy
  up <name>      create the agent network and start its container
  down <name>    stop and remove the container and network
  status <name>  report whether the agent container is running
  ls             list agent directories and their status

  up/down flags: --engine docker|podman|auto, --dry-run
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
	switch args[0] {
	case "init":
		return runAgentInit(ctx, a, args[1:], stdout, stderr)
	case "up", "down":
		return runAgentLifecycle(ctx, a, args[0], args[1:], stdout, stderr)
	case "status":
		return runAgentStatus(ctx, a, args[1:], stdout, stderr)
	case "ls":
		return runAgentList(ctx, a, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "agent: unknown subcommand %s\n\n%s", args[0], agentUsage)
		return 2
	}
}

func runAgentInit(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || !validAgentName(args[0]) {
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
	if err := initAgent(cwd, args[0], stderr); err != nil {
		fmt.Fprintf(stderr, "agent init: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "iugum agent: created %s/agent.yaml, home/, and data/\n", args[0])
	return 0
}

type agentLifecycleOpts struct {
	Name   string
	Engine string
	DryRun bool
}

func parseAgentLifecycleArgs(verb string, args []string, stderr io.Writer) (agentLifecycleOpts, bool) {
	var o agentLifecycleOpts
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			o.DryRun = true
		case arg == "--engine":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "agent %s: --engine requires a value\n", verb)
				return o, false
			}
			i++
			o.Engine = args[i]
		case strings.HasPrefix(arg, "--engine="):
			o.Engine = strings.TrimPrefix(arg, "--engine=")
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "agent %s: unknown flag %s\n", verb, arg)
			return o, false
		case o.Name == "":
			o.Name = arg
		default:
			fmt.Fprintf(stderr, "agent %s: extra argument %s\n", verb, arg)
			return o, false
		}
	}
	if !validAgentName(o.Name) {
		fmt.Fprintf(stderr, "Usage: iugum agent %s <name> [--engine E] [--dry-run]\n", verb)
		return o, false
	}
	return o, true
}

func runAgentLifecycle(ctx context.Context, a *app.App, verb string, args []string, stdout, stderr io.Writer) int {
	o, ok := parseAgentLifecycleArgs(verb, args, stderr)
	if !ok {
		return 2
	}
	action := map[string]string{"up": "run", "down": "stop"}[verb]
	if err := a.Check(ctx, "agent", action); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	root, cfg, err := loadNamedAgent(o.Name)
	if err != nil {
		fmt.Fprintf(stderr, "agent %s: %v\n", verb, err)
		return 1
	}
	if cfg.Network.Mode == "locked" {
		fmt.Fprintf(stderr, "agent %s: network mode locked is reserved; use open\n", verb)
		return 2
	}
	engine, err := resolveEngine(o.Engine, "", o.DryRun)
	if err != nil {
		fmt.Fprintf(stderr, "agent %s: %v\n", verb, err)
		return 2
	}
	if verb == "up" {
		return agentUp(engine, root, cfg, o.DryRun, stdout, stderr)
	}
	return agentDown(engine, cfg, o.DryRun, stdout, stderr)
}

func loadNamedAgent(name string) (string, AgentFile, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", AgentFile{}, err
	}
	root := filepath.Join(cwd, name)
	cfg, err := LoadAgentFile(filepath.Join(root, "agent.yaml"))
	if err != nil {
		return "", AgentFile{}, err
	}
	if cfg.Name == "" {
		cfg.Name = name
	}
	if !validAgentName(cfg.Name) {
		return "", AgentFile{}, fmt.Errorf("invalid name %q in agent.yaml", cfg.Name)
	}
	if cfg.Image == "" {
		return "", AgentFile{}, errors.New("image is required in agent.yaml")
	}
	return root, cfg, nil
}

func agentNetworkName(cfg AgentFile) string {
	name := cfg.Network.Name
	if name == "" || name == cfg.Name {
		return "iugum-agent-" + cfg.Name
	}
	return "iugum-agent-" + cfg.Name + "-" + name
}

func agentRunArgv(engine, root string, cfg AgentFile) []string {
	argv := []string{
		engine, "run", "-d",
		"--name", cfg.Name,
		"--network", agentNetworkName(cfg),
		"--restart", cfg.Startup.Restart,
		"--user", "1000:1000",
	}
	for _, m := range cfg.Mounts {
		if m.Target == "" {
			continue
		}
		if m.Tmpfs {
			value := m.Target
			if m.RO {
				value += ":ro"
			}
			argv = append(argv, "--tmpfs", value)
			continue
		}
		source := m.Source
		if !filepath.IsAbs(source) {
			source = filepath.Join(root, source)
		}
		value := filepath.Clean(source) + ":" + m.Target
		if m.RO {
			value += ":ro"
		}
		argv = append(argv, "-v", value)
	}
	for _, port := range cfg.Ports {
		argv = append(argv, "-p", port)
	}
	for _, name := range cfg.Startup.Env {
		argv = append(argv, "-e", name)
	}
	if cfg.Privileges != nil {
		for _, capability := range cfg.Privileges.CapAdd {
			argv = append(argv, "--cap-add", capability)
		}
	}
	return append(argv, cfg.Image)
}

func agentUp(engine, root string, cfg AgentFile, dryRun bool, stdout, stderr io.Writer) int {
	network := agentNetworkName(cfg)
	if dryRun {
		fmt.Fprintln(stdout, strings.Join([]string{engine, "network", "create", network}, " "))
		fmt.Fprintln(stdout, strings.Join(agentRunArgv(engine, root, cfg), " "))
		return 0
	}
	if !agentObjectExists(engine, "network", network) {
		if code := execArgv([]string{engine, "network", "create", network}, false); code != 0 {
			return code
		}
	}
	if agentObjectExists(engine, "container", cfg.Name) {
		if agentRunning(engine, cfg.Name) {
			fmt.Fprintf(stdout, "agent %s: already running\n", cfg.Name)
			return 0
		}
		return execArgv([]string{engine, "start", cfg.Name}, false)
	}
	return execArgv(agentRunArgv(engine, root, cfg), false)
}

func agentDown(engine string, cfg AgentFile, dryRun bool, stdout, stderr io.Writer) int {
	commands := [][]string{
		{engine, "stop", cfg.Name},
		{engine, "rm", cfg.Name},
		{engine, "network", "rm", agentNetworkName(cfg)},
	}
	if dryRun {
		for _, argv := range commands {
			fmt.Fprintln(stdout, strings.Join(argv, " "))
		}
		return 0
	}
	if agentObjectExists(engine, "container", cfg.Name) {
		for _, argv := range commands[:2] {
			if code := execArgv(argv, false); code != 0 {
				return code
			}
		}
	}
	if agentObjectExists(engine, "network", agentNetworkName(cfg)) {
		if code := execArgv(commands[2], false); code != 0 {
			fmt.Fprintf(stderr, "agent down: network %s is still in use\n", agentNetworkName(cfg))
			return code
		}
	}
	return 0
}

func agentObjectExists(engine, kind, name string) bool {
	args := []string{"inspect", name}
	if kind == "network" {
		args = []string{"network", "inspect", name}
	}
	cmd := exec.Command(engine, args...)
	return cmd.Run() == nil
}

func agentRunning(engine, name string) bool {
	cmd := exec.Command(engine, "inspect", "--format", "{{.State.Running}}", name)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func runAgentStatus(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || !validAgentName(args[0]) {
		fmt.Fprintln(stderr, "Usage: iugum agent status <name>")
		return 2
	}
	if err := a.Check(ctx, "agent", "status"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	_, cfg, err := loadNamedAgent(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agent status: %v\n", err)
		return 1
	}
	engine, err := resolveEngine("", "", false)
	if err != nil || !agentRunning(engine, cfg.Name) {
		fmt.Fprintf(stdout, "%s not-running\n", args[0])
		return 0
	}
	fmt.Fprintf(stdout, "%s running\n", args[0])
	return 0
}

func runAgentList(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "Usage: iugum agent ls")
		return 2
	}
	if err := a.Check(ctx, "agent", "ls"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		fmt.Fprintf(stderr, "agent ls: %v\n", err)
		return 1
	}
	engine, engineErr := resolveEngine("", "", false)
	for _, entry := range entries {
		if !entry.IsDir() || !validAgentName(entry.Name()) {
			continue
		}
		cfg, err := LoadAgentFile(filepath.Join(entry.Name(), "agent.yaml"))
		if err != nil {
			continue
		}
		status := "not-running"
		if engineErr == nil && agentRunning(engine, cfg.Name) {
			status = "running"
		}
		fmt.Fprintf(stdout, "%s\t%s\n", entry.Name(), status)
	}
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
