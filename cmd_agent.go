package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/srhopkins/iugum/agenthome"
	"github.com/srhopkins/iugum/app"
	"gopkg.in/yaml.v3"
)

const agentUsage = `Usage: iugum agent <run|clone|session|init|up|down|status|ls|tui|acp|rm|checkpoint>

  supervisor --home DIRECTORY
                 print launchd configuration (does not install or start it)
  policy-apply --home DIRECTORY --file POLICY.csv [--yes]
                 review and explicitly approve replacing instance policy

  start|stop|attach|status --home DIRECTORY
                 manage or attach to a native background agent

  home-init NAME DIRECTORY
                 create a private native agent home with no permission grants

  run --config FILE
                 run a native agent and its local chat workspace

  clone --config FILE --name NAME --output DIR
                 create a named candidate with separate writable state

  init <name>    create agent.yaml, home/, data/, probes, and starter policy
  up <name>      create the agent network and start its container; works with
                 no ./NAME/agent.yaml when given --kind or --image
  down <name>    stop and remove the container and network (keeps volumes)
  status <name>  report whether the agent container is running
  ls             list agent directories, merged with running iugum-managed
                 containers found by Docker/Podman label
  tui <name>     attach an interactive OpenCode terminal
  shell <name>   open a bash shell in the container as the agent user (uid 1000)
  acp <name>     bridge OpenCode ACP over stdin and stdout
  rm <name>      stop and remove the container, network, and its volumes;
                 asks for the name again on stdin unless --yes is given
  checkpoint <name>
                 checkpoint and commit the agent memory database

  up flags:       --engine docker|podman|auto, --dry-run, --kind KIND,
                  --image IMG, --label K=V (repeatable), --volume SPEC
                  (repeatable), --network NAME, --shm-size SIZE,
                  --env K=V (repeatable), --user USER, --mem SIZE,
                  --cpus N, --port SPEC (repeatable)
  down flags:     --engine docker|podman|auto, --dry-run
  rm flags:       --engine docker|podman|auto, --dry-run, --yes
  tui/acp flags:  --dry-run

  With no ./NAME/agent.yaml, agent.yaml values are skipped and --kind or
  --image is required. Precedence for up: --kind preset < agent.yaml (if
  present) < flags. Flags for --label/--volume/--env/--port add to the
  merged list; other flags replace the value.
`

const starterAgentPolicy = `# Casbin policy: subject, object, action, effect
# Default allows all, including iugum job add/rm/run.
# Lock cron for this agent by uncommenting the deny rows:
# p, *, schedule, add, deny
# p, *, schedule, remove, deny
# p, *, schedule, run, deny
p, *, *, *, allow
`

const starterAgentIugum = `actor: %s
data_dir: /data
tracker: beads
wiki: silverbullet
observe: memory
memory: sqlite
policy:
  policy: /home/iugum/policy.csv
`

const starterAgentJobs = `jobs: []
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
	case "supervisor", "policy-apply":
		return runAgentAdmin(ctx, a, args[0], args[1:], stdout, stderr)
	case "start", "stop", "attach":
		return runAgentProcess(ctx, a, args[0], args[1:], stdout, stderr)
	case "home-init":
		if len(args) != 3 {
			fmt.Fprintln(stderr, "Usage: iugum agent home-init NAME DIRECTORY")
			return 2
		}
		if err := a.Check(ctx, "agent", "init"); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := agenthome.Init(args[2], args[1]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, "Created agent home:", args[2])
		return 0
	case "session":
		return runAgentSession(ctx, a, args[1:], stdout, stderr)
	case "clone":
		return runAgentClone(ctx, a, args[1:], stdout, stderr)
	case "run":
		return runNativeAgent(ctx, a, args[1:], stdout, stderr)
	case "init":
		return runAgentInit(ctx, a, args[1:], stdout, stderr)
	case "up":
		return runAgentUp(ctx, a, args[1:], stdout, stderr)
	case "down":
		return runAgentLifecycle(ctx, a, args[0], args[1:], stdout, stderr)
	case "rm":
		return runAgentRm(ctx, a, args[1:], os.Stdin, stdout, stderr)
	case "status":
		if len(args) > 1 && strings.HasPrefix(args[1], "--home") {
			return runAgentProcess(ctx, a, "status", args[1:], stdout, stderr)
		}
		return runAgentStatus(ctx, a, args[1:], stdout, stderr)
	case "ls":
		return runAgentList(ctx, a, args[1:], stdout, stderr)
	case "tui", "acp", "shell":
		return runAgentAttach(ctx, a, args[0], args[1:], stdout, stderr)
	case "checkpoint":
		return runAgentCheckpoint(ctx, a, args[1:], stdout, stderr)
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

// agentUpOpts holds the flags for "agent up", which can fully describe an
// agent without any agent.yaml on disk.
type agentUpOpts struct {
	Name    string
	Engine  string
	DryRun  bool
	Kind    string
	Image   string
	User    string
	Network string
	ShmSize string
	Mem     string
	Cpus    string
	Labels  []string
	Volumes []string
	Envs    []string
	Ports   []string
}

func parseAgentUpArgs(args []string, stderr io.Writer) (agentUpOpts, bool) {
	var o agentUpOpts
	usage := func() {
		fmt.Fprintln(stderr, "Usage: iugum agent up <name> [--engine E] [--dry-run] [--kind K] [--image IMG]\n"+
			"    [--label K=V]... [--volume SPEC]... [--network NAME] [--shm-size SIZE]\n"+
			"    [--env K=V]... [--user USER] [--mem SIZE] [--cpus N] [--port SPEC]...")
	}
	// value takes the flag's value, either "--flag=value" or the next argv.
	value := func(i *int, flag string) (string, bool) {
		arg := args[*i]
		if strings.HasPrefix(arg, flag+"=") {
			return strings.TrimPrefix(arg, flag+"="), true
		}
		if *i+1 >= len(args) {
			fmt.Fprintf(stderr, "agent up: %s requires a value\n", flag)
			return "", false
		}
		*i++
		return args[*i], true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		flag := arg
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			flag = arg[:idx]
		}
		switch flag {
		case "--dry-run":
			o.DryRun = true
		case "--engine":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Engine = v
		case "--kind":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Kind = v
		case "--image":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Image = v
		case "--user":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.User = v
		case "--network":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Network = v
		case "--shm-size":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.ShmSize = v
		case "--mem":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Mem = v
		case "--cpus":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Cpus = v
		case "--label":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Labels = append(o.Labels, v)
		case "--volume":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Volumes = append(o.Volumes, v)
		case "--env":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Envs = append(o.Envs, v)
		case "--port":
			v, ok := value(&i, flag)
			if !ok {
				return o, false
			}
			o.Ports = append(o.Ports, v)
		case "--help", "-h":
			usage()
			return o, false
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "agent up: unknown flag %s\n", arg)
				return o, false
			}
			if o.Name == "" {
				o.Name = arg
			} else {
				fmt.Fprintf(stderr, "agent up: extra argument %s\n", arg)
				return o, false
			}
		}
	}
	if !validAgentName(o.Name) {
		usage()
		return o, false
	}
	return o, true
}

// detectDockerContext resolves the Docker context used to pick a kind
// preset's routing labels: DOCKER_CONTEXT if set, else "<engine> context
// show" for docker, else "homelab" for podman. A dry run with DOCKER_CONTEXT
// set never shells out, so it works without docker installed.
func detectDockerContext(engine string, dryRun bool) string {
	if v := os.Getenv("DOCKER_CONTEXT"); v != "" {
		return v
	}
	if engine == "podman" {
		return "homelab"
	}
	out, err := exec.Command(engine, "context", "show").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// overlayAgentFile applies agent.yaml's explicitly-set fields on top of a
// kind preset. Scalars replace; list fields replace wholesale (flags are the
// only layer that appends).
func overlayAgentFile(base, over AgentFile) AgentFile {
	if over.Name != "" {
		base.Name = over.Name
	}
	if over.Image != "" {
		base.Image = over.Image
	}
	if over.Kind != "" {
		base.Kind = over.Kind
	}
	if over.User != "" {
		base.User = over.User
	}
	if len(over.Labels) > 0 {
		base.Labels = over.Labels
	}
	if len(over.Mounts) > 0 {
		base.Mounts = over.Mounts
	}
	if len(over.Volumes) > 0 {
		base.Volumes = over.Volumes
	}
	if len(over.Ports) > 0 {
		base.Ports = over.Ports
	}
	if over.Network.External != "" {
		base.Network.External = over.Network.External
	}
	if over.Network.Name != "" {
		base.Network.Name = over.Network.Name
	}
	if over.Network.Mode != "" {
		base.Network.Mode = over.Network.Mode
	}
	if over.Privileges != nil {
		base.Privileges = over.Privileges
	}
	if over.Startup.Restart != "" {
		base.Startup.Restart = over.Startup.Restart
	}
	if len(over.Startup.Env) > 0 {
		base.Startup.Env = over.Startup.Env
	}
	if len(over.Startup.Command) > 0 {
		base.Startup.Command = over.Startup.Command
	}
	if over.Jobs != "" {
		base.Jobs = over.Jobs
	}
	if over.ShmSize != "" {
		base.ShmSize = over.ShmSize
	}
	if len(over.ExtraHosts) > 0 {
		base.ExtraHosts = over.ExtraHosts
	}
	if over.Mem != "" {
		base.Mem = over.Mem
	}
	if over.Cpus != "" {
		base.Cpus = over.Cpus
	}
	if len(over.Env) > 0 {
		base.Env = over.Env
	}
	return base
}

// applyAgentUpFlags layers up's flags on top of the merged kindPreset +
// agent.yaml config. List flags (labels/volumes/env/ports) append; the rest
// replace the merged value.
func applyAgentUpFlags(cfg AgentFile, o agentUpOpts) AgentFile {
	if o.Image != "" {
		cfg.Image = o.Image
	}
	if o.User != "" {
		cfg.User = o.User
	}
	if o.Network != "" {
		cfg.Network.External = o.Network
	}
	if o.ShmSize != "" {
		cfg.ShmSize = o.ShmSize
	}
	if o.Mem != "" {
		cfg.Mem = o.Mem
	}
	if o.Cpus != "" {
		cfg.Cpus = o.Cpus
	}
	cfg.Labels = append(cfg.Labels, o.Labels...)
	cfg.Volumes = append(cfg.Volumes, o.Volumes...)
	cfg.Env = append(cfg.Env, o.Envs...)
	cfg.Ports = append(cfg.Ports, o.Ports...)
	return cfg
}

// resolveAgentUpConfig merges the kindPreset, ./NAME/agent.yaml (if present),
// and up's flags, in that precedence, into the AgentFile agentUp will run.
func resolveAgentUpConfig(name string, o agentUpOpts, engine string) (string, AgentFile, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", AgentFile{}, err
	}
	root := filepath.Join(cwd, name)
	yamlPath := filepath.Join(root, "agent.yaml")
	var fileCfg AgentFile
	hasFile := false
	if _, statErr := os.Stat(yamlPath); statErr == nil {
		hasFile = true
		fileCfg, err = LoadAgentFile(yamlPath)
		if err != nil {
			return "", AgentFile{}, err
		}
	}

	kind := o.Kind
	if kind == "" {
		kind = fileCfg.Kind
	}

	base := AgentFile{Name: name}
	if kind != "" {
		dockerContext := detectDockerContext(engine, o.DryRun)
		base, err = kindPreset(kind, dockerContext, name)
		if err != nil {
			return "", AgentFile{}, err
		}
	}

	if hasFile {
		base = overlayAgentFile(base, fileCfg)
	}
	base.Name = name
	base = applyAgentUpFlags(base, o)
	base.applyDefaults()

	if base.Image == "" {
		return "", AgentFile{}, fmt.Errorf("agent up: no ./%s/agent.yaml found; specify --image or --kind (known kinds: %v)", name, kindNames())
	}
	if !validAgentName(base.Name) {
		return "", AgentFile{}, fmt.Errorf("invalid name %q", base.Name)
	}
	return root, base, nil
}

func runAgentUp(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	o, ok := parseAgentUpArgs(args, stderr)
	if !ok {
		return 2
	}
	if err := a.Check(ctx, "agent", "run"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	engine, err := resolveEngine(o.Engine, "", o.DryRun)
	if err != nil {
		fmt.Fprintf(stderr, "agent up: %v\n", err)
		return 2
	}
	root, cfg, err := resolveAgentUpConfig(o.Name, o, engine)
	if err != nil {
		fmt.Fprintf(stderr, "agent up: %v\n", err)
		return 1
	}
	if cfg.Network.Mode == "locked" {
		fmt.Fprintln(stderr, "agent up: network mode locked is reserved; use open")
		return 2
	}
	return agentUp(engine, root, cfg, o.DryRun, stdout, stderr)
}

func runAgentLifecycle(ctx context.Context, a *app.App, verb string, args []string, stdout, stderr io.Writer) int {
	o, ok := parseAgentLifecycleArgs(verb, args, stderr)
	if !ok {
		return 2
	}
	if err := a.Check(ctx, "agent", "stop"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	_, cfg, hasFile, err := loadNamedAgent(o.Name)
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
	if !hasFile && !o.DryRun {
		if _, ok := agentContainerByLabel(engine, cfg.Name); !ok {
			fmt.Fprintf(stderr, "agent %s: no container found with label iugum.agent=%s\n", verb, cfg.Name)
			return 1
		}
	}
	return agentDown(engine, cfg, o.DryRun, stdout, stderr)
}

// loadNamedAgent loads ./NAME/agent.yaml when present, and otherwise falls
// back to a minimal AgentFile so lookups (status/down/tui/acp/rm) can act on
// a container found by label alone. The bool reports whether agent.yaml was
// found.
func loadNamedAgent(name string) (string, AgentFile, bool, error) {
	if !validAgentName(name) {
		return "", AgentFile{}, false, fmt.Errorf("invalid name %q", name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", AgentFile{}, false, err
	}
	root := filepath.Join(cwd, name)
	yamlPath := filepath.Join(root, "agent.yaml")
	if _, statErr := os.Stat(yamlPath); errors.Is(statErr, os.ErrNotExist) {
		return root, AgentFile{Name: name}, false, nil
	}
	cfg, err := LoadAgentFile(yamlPath)
	if err != nil {
		return "", AgentFile{}, false, err
	}
	if cfg.Name == "" {
		cfg.Name = name
	}
	if !validAgentName(cfg.Name) {
		return "", AgentFile{}, false, fmt.Errorf("invalid name %q in agent.yaml", cfg.Name)
	}
	if cfg.Image == "" {
		return "", AgentFile{}, false, errors.New("image is required in agent.yaml")
	}
	return root, cfg, true, nil
}

// agentContainerByLabel finds a container by its iugum.agent label, since
// without agent.yaml the only source of truth is the container itself.
func agentContainerByLabel(engine, name string) (string, bool) {
	out, err := exec.Command(engine, "ps", "-a", "--filter", "label=iugum.agent="+name, "--format", "{{.Names}}").Output()
	if err != nil {
		return "", false
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

func agentNetworkName(cfg AgentFile) string {
	if cfg.Network.External != "" {
		return cfg.Network.External
	}
	name := cfg.Network.Name
	if name == "" || name == cfg.Name {
		return "iugum-agent-" + cfg.Name
	}
	return "iugum-agent-" + cfg.Name + "-" + name
}

// agentKind returns the agent's declared kind, or "custom" when unset.
func agentKind(cfg AgentFile) string {
	if cfg.Kind == "" {
		return "custom"
	}
	return cfg.Kind
}

// agentVolumeName extracts the leading "name" from a "name:target[:ro]" volume spec.
func agentVolumeName(spec string) string {
	name, _, _ := strings.Cut(spec, ":")
	return name
}

func agentRunArgv(engine, root string, cfg AgentFile) []string {
	argv := []string{
		engine, "run", "-d",
		"--name", cfg.Name,
		"--network", agentNetworkName(cfg),
		"--restart", cfg.Startup.Restart,
	}
	if cfg.User != "" {
		argv = append(argv, "--user", cfg.User)
	}
	for _, label := range cfg.Labels {
		argv = append(argv, "--label", label)
	}
	argv = append(argv,
		"--label", "iugum.managed=true",
		"--label", "iugum.agent="+cfg.Name,
		"--label", "iugum.image="+cfg.Image,
		"--label", "iugum.kind="+agentKind(cfg),
	)
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
	for _, v := range cfg.Volumes {
		if v == "" {
			continue
		}
		argv = append(argv, "-v", v)
	}
	for _, port := range cfg.Ports {
		argv = append(argv, "-p", port)
	}
	envFile := filepath.Join(root, "home", ".env")
	if _, err := os.Stat(envFile); err == nil {
		argv = append(argv, "--env-file", envFile)
	}
	for _, name := range cfg.Startup.Env {
		argv = append(argv, "-e", name)
	}
	for _, kv := range cfg.Env {
		if kv == "" {
			continue
		}
		argv = append(argv, "-e", kv)
	}
	if cfg.Mem != "" {
		argv = append(argv, "--memory", cfg.Mem, "--memory-swap", cfg.Mem)
	}
	if cfg.Cpus != "" {
		argv = append(argv, "--cpus", cfg.Cpus)
	}
	if cfg.ShmSize != "" {
		argv = append(argv, "--shm-size", cfg.ShmSize)
	}
	for _, host := range cfg.ExtraHosts {
		if host != "" {
			argv = append(argv, "--add-host", host)
		}
	}
	if cfg.Jobs != "" {
		source := cfg.Jobs
		if !filepath.IsAbs(source) {
			source = filepath.Join(root, source)
		}
		argv = append(argv, "-v", filepath.Clean(source)+":/workspace/jobs.yaml")
		argv = append(argv, "-e", "IUGUM_JOBS=/workspace/jobs.yaml")
	}
	if cfg.Privileges != nil {
		for _, capability := range cfg.Privileges.CapAdd {
			argv = append(argv, "--cap-add", capability)
		}
	}
	argv = append(argv, cfg.Image)
	if len(cfg.Startup.Command) > 0 {
		argv = append(argv, cfg.Startup.Command...)
	}
	return argv
}

// agentVolumeCreateArgv builds the argv that ensures a named volume exists,
// stamped with the same iugum.* labels the container gets.
func agentVolumeCreateArgv(engine, name string, cfg AgentFile) []string {
	return []string{
		engine, "volume", "create",
		"--label", "iugum.managed=true",
		"--label", "iugum.agent=" + cfg.Name,
		name,
	}
}

// agentNetworkCreateArgv builds the argv that creates an iugum-managed
// network, with labels before the trailing network name.
func agentNetworkCreateArgv(engine, name string, cfg AgentFile) []string {
	return []string{
		engine, "network", "create",
		"--label", "iugum.managed=true",
		"--label", "iugum.agent=" + cfg.Name,
		name,
	}
}

func agentUp(engine, root string, cfg AgentFile, dryRun bool, stdout, stderr io.Writer) int {
	network := agentNetworkName(cfg)
	external := cfg.Network.External != ""
	if dryRun {
		if !external {
			fmt.Fprintln(stdout, strings.Join(agentNetworkCreateArgv(engine, network, cfg), " "))
		}
		for _, v := range cfg.Volumes {
			name := agentVolumeName(v)
			if name == "" {
				continue
			}
			fmt.Fprintln(stdout, strings.Join(agentVolumeCreateArgv(engine, name, cfg), " "))
		}
		fmt.Fprintln(stdout, strings.Join(agentRunArgv(engine, root, cfg), " "))
		return 0
	}
	if !external && !agentObjectExists(engine, "network", network) {
		if code := execArgv(agentNetworkCreateArgv(engine, network, cfg), false); code != 0 {
			return code
		}
	}
	for _, v := range cfg.Volumes {
		name := agentVolumeName(v)
		if name == "" || agentObjectExists(engine, "volume", name) {
			continue
		}
		if code := execArgv(agentVolumeCreateArgv(engine, name, cfg), false); code != 0 {
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
	network := agentNetworkName(cfg)
	external := cfg.Network.External != ""
	commands := [][]string{
		{engine, "stop", cfg.Name},
		{engine, "rm", cfg.Name},
		{engine, "network", "rm", network},
	}
	if dryRun {
		for _, argv := range commands[:2] {
			fmt.Fprintln(stdout, strings.Join(argv, " "))
		}
		if !external {
			fmt.Fprintln(stdout, strings.Join(commands[2], " "))
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
	if !external && agentObjectExists(engine, "network", network) {
		if code := execArgv(commands[2], false); code != 0 {
			fmt.Fprintf(stderr, "agent down: network %s is still in use\n", network)
			return code
		}
	}
	return 0
}

type agentRmOpts struct {
	Name   string
	Engine string
	DryRun bool
	Yes    bool
}

func parseAgentRmArgs(args []string, stderr io.Writer) (agentRmOpts, bool) {
	var o agentRmOpts
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dry-run":
			o.DryRun = true
		case arg == "--yes":
			o.Yes = true
		case arg == "--engine":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "agent rm: --engine requires a value")
				return o, false
			}
			i++
			o.Engine = args[i]
		case strings.HasPrefix(arg, "--engine="):
			o.Engine = strings.TrimPrefix(arg, "--engine=")
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "agent rm: unknown flag %s\n", arg)
			return o, false
		case o.Name == "":
			o.Name = arg
		default:
			fmt.Fprintf(stderr, "agent rm: extra argument %s\n", arg)
			return o, false
		}
	}
	if !validAgentName(o.Name) {
		fmt.Fprintln(stderr, "Usage: iugum agent rm <name> [--engine E] [--dry-run] [--yes]")
		return o, false
	}
	return o, true
}

// agentRmVolumeNames lists the volumes rm should try to remove: every named
// volume in cfg.Volumes plus the "<name>-config" convention volume kind
// presets use, so a dry run without agent.yaml still shows something to do.
func agentRmVolumeNames(name string, cfg AgentFile) []string {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	add(name + "-config")
	for _, v := range cfg.Volumes {
		add(agentVolumeName(v))
	}
	return names
}

func runAgentRm(ctx context.Context, a *app.App, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, ok := parseAgentRmArgs(args, stderr)
	if !ok {
		return 2
	}
	if err := a.Check(ctx, "agent", "stop"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	_, cfg, hasFile, err := loadNamedAgent(o.Name)
	if err != nil {
		fmt.Fprintf(stderr, "agent rm: %v\n", err)
		return 1
	}
	if !o.Yes {
		fmt.Fprintf(stdout, "Type %s to confirm removing its container, network, and volumes: ", o.Name)
		reader := bufio.NewReader(stdin)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(line) != o.Name {
			fmt.Fprintln(stderr, "agent rm: confirmation did not match; aborted")
			return 1
		}
	}
	engine, err := resolveEngine(o.Engine, "", o.DryRun)
	if err != nil {
		fmt.Fprintf(stderr, "agent rm: %v\n", err)
		return 2
	}
	if !hasFile && !o.DryRun {
		if _, ok := agentContainerByLabel(engine, cfg.Name); !ok {
			fmt.Fprintf(stderr, "agent rm: no container found with label iugum.agent=%s\n", cfg.Name)
			return 1
		}
	}
	network := agentNetworkName(cfg)
	external := cfg.Network.External != ""
	volumes := agentRmVolumeNames(o.Name, cfg)
	if o.DryRun {
		fmt.Fprintln(stdout, strings.Join([]string{engine, "stop", cfg.Name}, " "))
		fmt.Fprintln(stdout, strings.Join([]string{engine, "rm", cfg.Name}, " "))
		if !external {
			fmt.Fprintln(stdout, strings.Join([]string{engine, "network", "rm", network}, " "))
		}
		for _, v := range volumes {
			fmt.Fprintln(stdout, strings.Join([]string{engine, "volume", "rm", v}, " "))
		}
		return 0
	}
	if agentObjectExists(engine, "container", cfg.Name) {
		if code := execArgv([]string{engine, "stop", cfg.Name}, false); code != 0 {
			return code
		}
		if code := execArgv([]string{engine, "rm", cfg.Name}, false); code != 0 {
			return code
		}
	}
	if !external && agentObjectExists(engine, "network", network) {
		if code := execArgv([]string{engine, "network", "rm", network}, false); code != 0 {
			fmt.Fprintf(stderr, "agent rm: network %s is still in use\n", network)
			return code
		}
	}
	out, err := exec.Command(engine, "volume", "ls", "-q", "--filter", "label=iugum.agent="+cfg.Name).Output()
	if err == nil {
		for _, v := range strings.Fields(string(out)) {
			execArgv([]string{engine, "volume", "rm", v}, false)
		}
	}
	return 0
}

func agentObjectExists(engine, kind, name string) bool {
	args := []string{"inspect", name}
	switch kind {
	case "network":
		args = []string{"network", "inspect", name}
	case "volume":
		args = []string{"volume", "inspect", name}
	}
	cmd := exec.Command(engine, args...)
	return cmd.Run() == nil
}

func agentRunning(engine, name string) bool {
	cmd := exec.Command(engine, "inspect", "--format", "{{.State.Running}}", name)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func agentAttachArgv(engine, name, verb string) []string {
	return agentAttachArgvUser(engine, name, verb, "")
}

// agentAttachArgvUser builds the exec argv. shell runs as the agent user, not
// the image default: linuxserver images start their init as root, so a plain
// `docker exec` lands in a root shell that then writes root-owned files into
// the user's home.
func agentAttachArgvUser(engine, name, verb, user string) []string {
	switch verb {
	case "tui":
		return []string{engine, "exec", "-it", name, "opencode"}
	case "shell":
		if user == "" {
			user = "1000:1000"
		}
		return []string{engine, "exec", "-it", "-u", user, name, "bash", "-l"}
	}
	return []string{engine, "exec", "-i", name, "opencode", "acp"}
}

func runAgentAttach(ctx context.Context, a *app.App, verb string, args []string, stdout, stderr io.Writer) int {
	o, ok := parseAgentLifecycleArgs(verb, args, stderr)
	if !ok {
		return 2
	}
	if o.Engine != "" {
		fmt.Fprintf(stderr, "agent %s: --engine is not supported\n", verb)
		return 2
	}
	if err := a.Check(ctx, "agent", verb); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	_, cfg, hasFile, err := loadNamedAgent(o.Name)
	if err != nil {
		fmt.Fprintf(stderr, "agent %s: %v\n", verb, err)
		return 1
	}
	engine, err := resolveEngine("", "", o.DryRun)
	if err != nil {
		fmt.Fprintf(stderr, "agent %s: %v\n", verb, err)
		return 2
	}
	if !hasFile && !o.DryRun {
		if _, ok := agentContainerByLabel(engine, cfg.Name); !ok {
			fmt.Fprintf(stderr, "agent %s: no container found with label iugum.agent=%s\n", verb, cfg.Name)
			return 1
		}
	}
	argv := agentAttachArgvUser(engine, cfg.Name, verb, cfg.User)
	if o.DryRun {
		fmt.Fprintln(stdout, strings.Join(argv, " "))
		return 0
	}
	if !agentRunning(engine, cfg.Name) {
		fmt.Fprintf(stderr, "agent %s: %s is not running\n", verb, cfg.Name)
		return 1
	}
	return execArgv(argv, false)
}

func runAgentCheckpoint(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || !validAgentName(args[0]) {
		fmt.Fprintln(stderr, "Usage: iugum agent checkpoint <name>")
		return 2
	}
	if err := a.Check(ctx, "agent", "checkpoint"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	root, _, hasFile, err := loadNamedAgent(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agent checkpoint: %v\n", err)
		return 1
	}
	if !hasFile {
		fmt.Fprintf(stderr, "agent checkpoint: no ./%s/agent.yaml found\n", args[0])
		return 1
	}
	if err := checkpointAgentMemory(root, args[0], "sqlite3", stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "agent checkpoint: %v\n", err)
		return 1
	}
	return 0
}

func checkpointAgentMemory(agentRoot, name, sqliteBin string, stdout, stderr io.Writer) error {
	memoryPath := filepath.Join(agentRoot, "home", "memory.db")
	if _, err := os.Stat(memoryPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stdout, "agent checkpoint: %s has no memory database; skipped\n", name)
		return nil
	} else if err != nil {
		return err
	}

	checkpoint := exec.Command(sqliteBin, memoryPath, "PRAGMA wal_checkpoint(TRUNCATE);")
	checkpoint.Stdout = io.Discard
	checkpoint.Stderr = stderr
	if err := checkpoint.Run(); err != nil {
		return fmt.Errorf("sqlite3: %w", err)
	}

	repoRoot, err := gitOutput(filepath.Dir(agentRoot), "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("find enclosing git repo: %w", err)
	}
	repoRoot, err = filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve git repo: %w", err)
	}
	resolvedMemory, err := filepath.EvalSymlinks(memoryPath)
	if err != nil {
		return fmt.Errorf("resolve memory database: %w", err)
	}
	relMemory, err := filepath.Rel(repoRoot, resolvedMemory)
	if err != nil || relMemory == ".." || strings.HasPrefix(relMemory, ".."+string(os.PathSeparator)) {
		return errors.New("memory database is outside the enclosing git repo")
	}
	if err := runAgentCommand(repoRoot, stderr, "git", "add", "--", relMemory); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	changed := exec.Command("git", "diff", "--cached", "--quiet", "--", relMemory)
	changed.Dir = repoRoot
	if err := changed.Run(); err == nil {
		fmt.Fprintf(stdout, "agent checkpoint: %s memory has no changes to commit\n", name)
		return nil
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return fmt.Errorf("git diff: %w", err)
		}
	}

	message := fmt.Sprintf("checkpoint %s agent memory", name)
	if err := runAgentCommand(repoRoot, stderr, "git", "commit", "--only", "-m", message, "--", relMemory); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	fmt.Fprintf(stdout, "agent checkpoint: committed %s\n", relMemory)
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func runAgentCommand(dir string, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	return cmd.Run()
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
	_, cfg, hasFile, err := loadNamedAgent(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "agent status: %v\n", err)
		return 1
	}
	engine, err := resolveEngine("", "", false)
	if err != nil {
		fmt.Fprintf(stdout, "%s not-running\n", args[0])
		return 0
	}
	if !hasFile {
		if _, ok := agentContainerByLabel(engine, cfg.Name); !ok {
			fmt.Fprintf(stdout, "%s not-running\n", args[0])
			return 0
		}
	}
	if !agentRunning(engine, cfg.Name) {
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
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() || !validAgentName(entry.Name()) {
			continue
		}
		cfg, err := LoadAgentFile(filepath.Join(entry.Name(), "agent.yaml"))
		if err != nil {
			continue
		}
		seen[cfg.Name] = true
		status := "not-running"
		if engineErr == nil && agentRunning(engine, cfg.Name) {
			status = "running"
		}
		fmt.Fprintf(stdout, "%s\t%s\n", entry.Name(), status)
	}
	if engineErr == nil {
		for _, line := range agentListManagedContainers(engine) {
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) == 0 || fields[0] == "" || seen[fields[0]] {
				continue
			}
			status := "not-running"
			if len(fields) > 2 && strings.HasPrefix(fields[2], "Up") {
				status = "running"
			}
			fmt.Fprintf(stdout, "%s\t%s\n", fields[0], status)
		}
	}
	return 0
}

// agentListManagedContainers lists every container carrying the
// iugum.managed=true label, so "ls" can show containers started with no
// agent.yaml on disk alongside agent directories.
func agentListManagedContainers(engine string) []string {
	out, err := exec.Command(engine, "ps", "-a",
		"--filter", "label=iugum.managed=true",
		"--format", `{{.Names}}\t{{.Label "iugum.kind"}}\t{{.Status}}`,
	).Output()
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
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
		Jobs:    "jobs.yaml",
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
		filepath.Join(data, "iugum.yaml"):   fmt.Sprintf(starterAgentIugum, name),
		filepath.Join(root, "jobs.yaml"):    starterAgentJobs,
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
