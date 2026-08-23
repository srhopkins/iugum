package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	_ "github.com/srhopkins/iugum/defaults"
	"github.com/srhopkins/iugum/embedbin"
	"github.com/srhopkins/iugum/ship"
)

//go:embed silverbullet/silverbullet
var silverbulletBin []byte

func init() {
	embedbin.Set(silverbulletBin)
}

const usage = `Usage: iugum <beads|wiki|run|prepare-pr|skill>

  beads        work-graph slot (default: beads)
  wiki         notes-server slot (default: SilverBullet)
  run          start jobs, file watch, and optional HTTP POST /hooks/{name}
  prepare-pr   write review files; do not push
  skill run    run a skill by name (prepare-pr)

  iugum beads [bd args...]
  iugum wiki [--port N] [--hostname ADDR] [space-dir]
  iugum prepare-pr [--repo DIR] [--base main] [--head BRANCH] [--title T] [--body-file F]
  iugum skill run prepare-pr [same flags]

  Body: stdin, or --body-file, or --body.
  Empty origin: writes push.md + push.sh.
  Origin has a branch: writes pr.md + create.sh.
  Every command passes Casbin (policy engine) first. Default model allows all.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stdout, usage)
		return 0
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iugum: %v\n", err)
		return 1
	}
	ctx := context.Background()

	switch args[0] {
	case "run":
		return runRuntime(ctx, a, cfg)
	case "beads":
		if err := a.RunTracker(ctx, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			return 1
		}
		return 0
	case "prepare-pr":
		return runPreparePR(ctx, a, args[1:])
	case "skill":
		if len(args) < 3 || args[1] != "run" {
			fmt.Fprintln(os.Stderr, "Usage: iugum skill run <name> [flags]")
			return 2
		}
		if args[2] != "prepare-pr" {
			fmt.Fprintf(os.Stderr, "Unknown skill: %s\n", args[2])
			return 2
		}
		return runPreparePR(ctx, a, args[3:])
	case "wiki":
		port, host, space, code, ok := parseWikiArgs(args[1:])
		if !ok {
			return code
		}
		if err := a.ServeWiki(ctx, contract.WikiOpts{Port: port, Host: host, Space: space}); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			if ee, ok := err.(interface{ ExitCode() int }); ok {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", args[0], usage)
		return 1
	}
}

func runRuntime(ctx context.Context, a *app.App, cfg config.File) int {
	for _, j := range cfg.Jobs {
		if len(j.Workflow) > 0 {
			a.Hooks.AddWorkflow(j.Name, j.Workflow)
		}
		spec := j.Spec
		if spec == "" {
			spec = "@triggered"
		}
		if err := a.Scheduler.Add(spec, j.Name, func(ctx context.Context, ev contract.Event) error {
			return a.FireHook(ctx, contract.Event{Name: j.Name, Source: "cron", Path: ev.Path, Attrs: ev.Attrs})
		}); err != nil {
			fmt.Fprintf(os.Stderr, "schedule: %v\n", err)
			return 1
		}
	}
	for _, h := range cfg.HookRoutes {
		a.Hooks.On(h.On, h.Job)
	}
	for _, w := range cfg.Watch {
		if w.Path == "" {
			continue
		}
		if err := a.Watcher.Add(w.Path); err != nil {
			fmt.Fprintf(os.Stderr, "watch: %v\n", err)
			return 1
		}
	}
	if err := a.Scheduler.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "schedule: %v\n", err)
		return 1
	}
	defer a.Scheduler.Stop()
	if cfg.HookHTTP != "" {
		if err := a.ListenHooksHTTP(ctx, cfg.HookHTTP); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	go func() {
		for ev := range a.Watcher.Events() {
			_ = a.FireHook(ctx, ev)
		}
	}()
	if cfg.HookHTTP != "" {
		fmt.Fprintf(os.Stdout, "iugum run: jobs + watch. HTTP POST /hooks/{name} on %s\n", cfg.HookHTTP)
	} else {
		fmt.Fprintln(os.Stdout, "iugum run: jobs + watch. HTTP listen off (set hook_http to bind).")
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	return 0
}

func runPreparePR(ctx context.Context, a *app.App, args []string) int {
	if err := a.Check(ctx, "ship", "prepare"); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	opts, code, ok := parsePrepareArgs(args)
	if !ok {
		return code
	}
	res, err := ship.Prepare(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare-pr: %v\n", err)
		return 1
	}
	fmt.Printf("kind=%s\nmd=%s\nsh=%s\n", res.Kind, res.MD, res.SH)
	return 0
}

func parsePrepareArgs(args []string) (opts ship.Opts, code int, ok bool) {
	opts.Stdin = os.Stdin
	for i := 0; i < len(args); i++ {
		a := args[i]
		need := func(flag string) (string, bool) {
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "prepare-pr: %s requires a value\n", flag)
				return "", false
			}
			i++
			return args[i], true
		}
		switch {
		case a == "--help" || a == "-h":
			fmt.Fprint(os.Stdout, usage)
			return opts, 0, false
		case a == "--repo":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.Repo = v
		case a == "--base":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.Base = v
		case a == "--head":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.Head = v
		case a == "--title" || a == "-t":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.Title = v
		case a == "--body" || a == "-b":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.Body = v
		case a == "--body-file" || a == "-F":
			v, good := need(a)
			if !good {
				return opts, 2, false
			}
			opts.BodyFile = v
		default:
			fmt.Fprintf(os.Stderr, "prepare-pr: unknown flag %s\n", a)
			return opts, 2, false
		}
	}
	return opts, 0, true
}

func parseWikiArgs(args []string) (port int, host, space string, code int, ok bool) {
	port = 3000
	host = "127.0.0.1"
	space = "./wiki"
	sawSpace := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			fmt.Fprint(os.Stdout, usage)
			return 0, "", "", 0, false
		case a == "--port" || a == "-p":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "wiki: --port requires a value")
				return 0, "", "", 2, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "wiki: invalid port %q\n", args[i])
				return 0, "", "", 2, false
			}
			port = n
		case strings.HasPrefix(a, "--port="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--port="))
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "wiki: invalid port %q\n", a)
				return 0, "", "", 2, false
			}
			port = n
		case a == "--hostname" || a == "-L":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "wiki: --hostname requires a value")
				return 0, "", "", 2, false
			}
			i++
			host = args[i]
		case strings.HasPrefix(a, "--hostname="):
			host = strings.TrimPrefix(a, "--hostname=")
			if host == "" {
				fmt.Fprintln(os.Stderr, "wiki: --hostname requires a value")
				return 0, "", "", 2, false
			}
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "wiki: unknown flag %s\n", a)
			return 0, "", "", 2, false
		default:
			if sawSpace {
				fmt.Fprintf(os.Stderr, "wiki: extra argument %s\n", a)
				return 0, "", "", 2, false
			}
			space = a
			sawSpace = true
		}
	}
	return port, host, space, 0, true
}
