package main

// iugum up: one command, two placements.
// Host mode runs wiki, observe, jobs/hooks/watch, and code-server in this process.
// Container mode runs the same inside the iugum image with docker or podman.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/dirs"
)

// lookPath finds a binary on PATH. Tests replace it.
var lookPath = exec.LookPath

// upOpts is the parsed `iugum up` command line.
type upOpts struct {
	// host mode
	WikiPort       int
	ObservePort    int
	CodeServerPort int
	NoCodeServer   bool
	BrowserPort    int
	NoBrowser      bool
	TtydPort       int
	NoTtyd         bool
	Host           string
	Space          string
	// container mode
	Container bool
	Image     string
	Engine    string
	Name      string
	Detach    bool
	DryRun    bool
}

// containerOpts is the parsed `iugum container build|stop` command line.
type containerOpts struct {
	With       string
	CodeServer string // "1", "0", or empty = follow WITH
	Browser    string // "1", "0", or empty = follow WITH
	Tag    string
	Engine string
	Name   string
	DryRun bool
}

func runUp(ctx context.Context, a *app.App, cfg config.File, args []string) int {
	o, code, ok := parseUpArgs(args)
	if !ok {
		return code
	}
	if o.Container {
		return runUpContainer(ctx, a, cfg, o)
	}
	return runUpHost(ctx, a, cfg, o)
}

// --- container mode ---

func runUpContainer(ctx context.Context, a *app.App, cfg config.File, o upOpts) int {
	if err := a.Check(ctx, "container", "run"); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	engine, err := resolveEngine(o.Engine, cfg.Container.Engine, o.DryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return 1
	}
	data, err := dirs.ResolveData(cfg.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return 1
	}
	argv := containerRunArgv(engine, cfg.Container, o, cwd, data)
	return execArgv(argv, o.DryRun)
}

// containerRunArgv builds `<engine> run ... IMAGE up`.
func containerRunArgv(engine string, c config.Container, o upOpts, cwd, data string) []string {
	image := firstOf(o.Image, c.Image, "iugum:latest")
	name := firstOf(o.Name, c.Name, "iugum")
	argv := []string{engine, "run", "--rm"}
	if o.Detach {
		argv = append(argv, "-d")
	}
	argv = append(argv, "--name", name)
	argv = append(argv, "-v", cwd+":/workspace", "-v", data+":/data")
	for _, m := range c.Mounts {
		argv = append(argv, "-v", m)
	}
	ports := c.Ports
	if len(ports) == 0 {
		ports = []string{"3000:3000", "3848:3848", "8080:8080", "7681:7681"}
	}
	for _, p := range ports {
		argv = append(argv, "-p", p)
	}
	argv = append(argv, "-e", "IUGUM_DATA=/data")
	for _, e := range c.Env {
		argv = append(argv, "-e", e)
	}
	argv = append(argv, "-w", "/workspace", image, "up")
	return argv
}

// containerBuildArgv builds `<engine> build --build-arg WITH=<list> -t <tag> .`.
func containerBuildArgv(engine string, c config.Container, o containerOpts) []string {
	tag := firstOf(o.Tag, c.Image, "iugum:latest")
	argv := []string{engine, "build"}
	if o.With != "" {
		argv = append(argv, "--build-arg", "WITH="+o.With)
	}
	if o.CodeServer != "" {
		argv = append(argv, "--build-arg", "CODE_SERVER="+o.CodeServer)
	}
	if o.Browser != "" {
		argv = append(argv, "--build-arg", "BROWSER="+o.Browser)
	}
	return append(argv, "-t", tag, ".")
}

// containerStopArgv builds `<engine> stop <name>`.
func containerStopArgv(engine string, c config.Container, o containerOpts) []string {
	return []string{engine, "stop", firstOf(o.Name, c.Name, "iugum")}
}

func runContainer(ctx context.Context, a *app.App, cfg config.File, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: iugum container <build|stop> [flags]")
		return 2
	}
	verb := args[0]
	if verb != "build" && verb != "stop" {
		fmt.Fprintf(os.Stderr, "container: unknown verb %s\n", verb)
		return 2
	}
	o, code, ok := parseContainerArgs(verb, args[1:])
	if !ok {
		return code
	}
	if err := a.Check(ctx, "container", verb); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	engine, err := resolveEngine(o.Engine, cfg.Container.Engine, o.DryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "container: %v\n", err)
		return 2
	}
	var argv []string
	if verb == "build" {
		argv = containerBuildArgv(engine, cfg.Container, o)
	} else {
		argv = containerStopArgv(engine, cfg.Container, o)
	}
	return execArgv(argv, o.DryRun)
}

// resolveEngine picks docker or podman. auto = first found on PATH.
// A dry run with nothing on PATH uses docker so the argv still prints.
func resolveEngine(flag, cfgEngine string, dryRun bool) (string, error) {
	name := firstOf(flag, cfgEngine, "auto")
	switch name {
	case "docker", "podman":
		return name, nil
	case "auto":
		for _, e := range []string{"docker", "podman"} {
			if _, err := lookPath(e); err == nil {
				return e, nil
			}
		}
		if dryRun {
			return "docker", nil
		}
		return "", errors.New("engine: no docker or podman on PATH")
	default:
		return "", fmt.Errorf("engine: unknown engine %q (use docker, podman, or auto)", name)
	}
}

// execArgv prints argv on dry run, else runs it with the terminal attached.
func execArgv(argv []string, dryRun bool) int {
	if dryRun {
		fmt.Fprintln(os.Stdout, strings.Join(argv, " "))
		return 0
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", argv[0], err)
		return 1
	}
	return 0
}

// --- host mode ---

func runUpHost(ctx context.Context, a *app.App, cfg config.File, o upOpts) int {
	if err := a.Check(ctx, "service", "serve"); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	// Network policy runs before any port binds. Backend off = no work.
	if err := a.ApplyNet(ctx); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	host := o.Host
	wikiPort, err := pickPort(host, o.WikiPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: wiki: %v\n", err)
		return 1
	}
	obsPort, err := pickPort(host, o.ObservePort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "up: observe: %v\n", err)
		return 1
	}
	codeBin := ""
	if !o.NoCodeServer && (cfg.Container.CodeServer == nil || *cfg.Container.CodeServer) {
		if p, err := lookPath("code-server"); err == nil {
			codeBin = p
		}
	}
	codePort := 0
	if codeBin != "" {
		codePort, err = pickPort(host, o.CodeServerPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "up: code-server: %v\n", err)
			return 1
		}
	}
	browserBin := ""
	if !o.NoBrowser {
		if p, err := lookPath("iugum-browser"); err == nil {
			if _, err := lookPath("chromium"); err == nil {
				browserBin = p
			} else if _, err := lookPath("chromium-browser"); err == nil {
				browserBin = p
			}
		}
	}
	browserPort := 0
	if browserBin != "" {
		browserPort, err = pickPort(host, o.BrowserPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "up: browser: %v\n", err)
			return 1
		}
	}
	ttydBin := ""
	if !o.NoTtyd {
		if p, err := lookPath("ttyd"); err == nil {
			ttydBin = p
		}
	}
	ttydPort := 0
	if ttydBin != "" {
		ttydPort, err = pickPort(host, o.TtydPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "up: ttyd: %v\n", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 8)
	var wg sync.WaitGroup
	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn(ctx)
			if err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("%s: %w", name, err)
				cancel()
			}
		}()
	}

	if err := startRuntime(ctx, a, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return 1
	}
	defer a.Scheduler.Stop()

	start("observe", func(c context.Context) error { return a.ServeObserve(c, obsPort, host) })
	fmt.Fprintf(os.Stdout, "observe http://%s\n", net.JoinHostPort(host, strconv.Itoa(obsPort)))

	start("wiki", func(c context.Context) error {
		return a.ServeWikiSupervised(c, contract.WikiOpts{Port: wikiPort, Host: host, Space: o.Space})
	})
	fmt.Fprintf(os.Stdout, "wiki http://%s\n", net.JoinHostPort(host, strconv.Itoa(wikiPort)))

	if codeBin != "" {
		cwd, _ := os.Getwd()
		start("code-server", func(c context.Context) error { return a.RunCodeServer(c, codeBin, host, codePort, cwd) })
		fmt.Fprintf(os.Stdout, "code-server http://%s\n", net.JoinHostPort(host, strconv.Itoa(codePort)))
	} else {
		fmt.Fprintln(os.Stdout, "code-server off")
	}
	if browserBin != "" {
		start("browser", func(c context.Context) error {
			return a.RunBrowser(c, browserBin, host, browserPort)
		})
		fmt.Fprintf(os.Stdout, "browser http://%s/\n", net.JoinHostPort(host, strconv.Itoa(browserPort)))
	} else {
		fmt.Fprintln(os.Stdout, "browser off")
	}
	if ttydBin != "" {
		cwd, _ := os.Getwd()
		start("ttyd", func(c context.Context) error { return a.RunTtyd(c, ttydBin, host, ttydPort, cwd) })
		fmt.Fprintf(os.Stdout, "ttyd http://%s/\n", net.JoinHostPort(host, strconv.Itoa(ttydPort)))
	} else {
		fmt.Fprintln(os.Stdout, "ttyd off")
	}
	if cfg.HookHTTP != "" {
		fmt.Fprintf(os.Stdout, "hooks http://%s/hooks/{name}\n", cfg.HookHTTP)
	}

	<-ctx.Done()
	wg.Wait()
	select {
	case err := <-errCh:
		fmt.Fprintf(os.Stderr, "up: %v\n", err)
		return 1
	default:
	}
	return 0
}

// startRuntime wires jobs, hook routes, file watch, and hook HTTP.
// It matches `iugum run` but returns instead of waiting for a signal.
func startRuntime(ctx context.Context, a *app.App, cfg config.File) error {
	if err := app.RegisterJobs(a, cfg.Jobs); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	for _, h := range cfg.HookRoutes {
		a.Hooks.On(h.On, h.Job)
	}
	for _, w := range cfg.Watch {
		if w.Path == "" {
			continue
		}
		if err := a.Watcher.Add(w.Path); err != nil {
			return fmt.Errorf("watch: %w", err)
		}
	}
	if err := a.Scheduler.Start(); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	if path := config.JobsFile(); path != "" {
		if err := app.WatchJobsFile(ctx, a, path); err != nil {
			return fmt.Errorf("jobs watch: %w", err)
		}
	}
	if cfg.HookHTTP != "" {
		if err := a.ListenHooksHTTP(ctx, cfg.HookHTTP); err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
	}
	go func() {
		for ev := range a.Watcher.Events() {
			_ = a.FireHook(ctx, ev)
		}
	}()
	return nil
}

// pickPort returns port, or a free port when port is 0.
func pickPort(host string, port int) (int, error) {
	if port != 0 {
		return port, nil
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// --- parsers ---

func parseUpArgs(args []string) (o upOpts, code int, ok bool) {
	o.WikiPort, o.ObservePort, o.CodeServerPort, o.BrowserPort, o.TtydPort = 3000, 3848, 8080, 6080, 7681
	o.Host = "127.0.0.1"
	o.Space = "./wiki"
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, val, hasEq := strings.Cut(a, "=")
		need := func() (string, bool) {
			if hasEq {
				return val, true
			}
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "up: %s requires a value\n", key)
				return "", false
			}
			i++
			return args[i], true
		}
		port := func(dst *int) bool {
			v, good := need()
			if !good {
				return false
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "up: invalid port %q\n", v)
				return false
			}
			*dst = n
			return true
		}
		str := func(dst *string) bool {
			v, good := need()
			if good {
				*dst = v
			}
			return good
		}
		switch key {
		case "--help", "-h":
			fmt.Fprint(os.Stdout, usage)
			return o, 0, false
		case "--container":
			o.Container = true
		case "--detach", "-d":
			o.Detach = true
		case "--dry-run":
			o.DryRun = true
		case "--no-code-server":
			o.NoCodeServer = true
		case "--no-browser":
			o.NoBrowser = true
		case "--no-ttyd":
			o.NoTtyd = true
		case "--wiki-port":
			if !port(&o.WikiPort) {
				return o, 2, false
			}
		case "--observe-port":
			if !port(&o.ObservePort) {
				return o, 2, false
			}
		case "--code-server-port":
			if !port(&o.CodeServerPort) {
				return o, 2, false
			}
		case "--browser-port":
			if !port(&o.BrowserPort) {
				return o, 2, false
			}
		case "--ttyd-port":
			if !port(&o.TtydPort) {
				return o, 2, false
			}
		case "--hostname", "-L":
			if !str(&o.Host) {
				return o, 2, false
			}
		case "--space":
			if !str(&o.Space) {
				return o, 2, false
			}
		case "--image":
			if !str(&o.Image) {
				return o, 2, false
			}
		case "--engine":
			if !str(&o.Engine) {
				return o, 2, false
			}
		case "--name":
			if !str(&o.Name) {
				return o, 2, false
			}
		default:
			fmt.Fprintf(os.Stderr, "up: unknown flag %s\n", a)
			return o, 2, false
		}
	}
	return o, 0, true
}

func parseContainerArgs(verb string, args []string) (o containerOpts, code int, ok bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		key, val, hasEq := strings.Cut(a, "=")
		need := func() (string, bool) {
			if hasEq {
				return val, true
			}
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "container %s: %s requires a value\n", verb, key)
				return "", false
			}
			i++
			return args[i], true
		}
		str := func(dst *string) bool {
			v, good := need()
			if good {
				*dst = v
			}
			return good
		}
		switch key {
		case "--help", "-h":
			fmt.Fprint(os.Stdout, usage)
			return o, 0, false
		case "--dry-run":
			o.DryRun = true
		case "--engine":
			if !str(&o.Engine) {
				return o, 2, false
			}
		case "--with":
			if verb != "build" || !str(&o.With) {
				return o, 2, false
			}
		case "--code-server":
			if verb != "build" || !str(&o.CodeServer) {
				return o, 2, false
			}
		case "--browser":
			if verb != "build" || !str(&o.Browser) {
				return o, 2, false
			}
		case "--tag", "-t":
			if verb != "build" || !str(&o.Tag) {
				return o, 2, false
			}
		case "--name":
			if verb != "stop" || !str(&o.Name) {
				return o, 2, false
			}
		default:
			fmt.Fprintf(os.Stderr, "container %s: unknown flag %s\n", verb, a)
			return o, 2, false
		}
	}
	return o, 0, true
}

func firstOf(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
