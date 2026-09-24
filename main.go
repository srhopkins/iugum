package main

import (
	"context"
	"embed"
	"fmt"
	"io"
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
	"github.com/srhopkins/iugum/spaceassets"
)

// THE BLOB DIRECTORY IS EMBEDDED, NOT THE BLOB FILE, AND THAT IS THE POINT.
//
// The server binary is 36MB of build output, so it is gitignored. But
// //go:embed is a COMPILE-TIME pattern and a pattern that matches nothing is a
// hard error, so a fresh clone or `git worktree add` could not build at all:
// "pattern silverbullet/silverbullet: no matching files found". Three agents
// and one session hit that in a single evening (iugum-ef0).
//
// The first workaround was worse than the problem: build-wiki-blob.sh wrote an
// empty 0644 placeholder so the compile would succeed, and `cp` onto an
// existing file keeps the DESTINATION's mode, so the copied server came out
// non-executable and the wiki refused to start.
//
// A DIRECTORY pattern only needs one match. wikiblob/PLACEHOLDER is tracked
// and supplies it, so the tree always compiles; the binary lands beside it and
// stays ignored. A build with no binary is then a clear runtime error naming
// the build script, instead of a compile failure or a silent empty file.
//
//go:embed wikiblob
var wikiblobFS embed.FS

// wikiblobName is the file the wiki server is installed as, inside wikiblobFS.
const wikiblobName = "wikiblob/silverbullet"

// silverbulletBinary returns the embedded server, or nil when this binary was
// built without one. nil is a legitimate state, not a fault to panic on: it is
// exactly what a fresh checkout produces, and the adapter turns it into a
// sentence a person can act on.
func silverbulletBinary() []byte {
	data, err := wikiblobFS.ReadFile(wikiblobName)
	if err != nil {
		return nil
	}
	return data
}

// The atomdown space assets that have another home in this repository. A
// space needs all three files: the two plug bundles, and the library page that
// carries the header-bar button (space-lua) and the card CSS (space-style).
// Without the library page the decorations land with no stylesheet, and the
// feature reads as broken rather than uninstalled.
//
// The plug YAML files are build inputs, and plugs/atomdown-e2e is a test
// suite, so neither one ships. Pages with no other home live in
// spaceassets/library and need no embed here.
var (
	//go:embed plugs/atomdown-board/atomdown-board.plug.js
	atomdownBoardPlug []byte
	//go:embed plugs/atomdown-inline/atomdown-inline.plug.js
	atomdownInlinePlug []byte
	//go:embed "plugs/atomdown-inline/library/Atomdown Inline.md"
	atomdownInlineLibrary []byte
)

func init() {
	embedbin.Set(silverbulletBinary())
	spaceassets.Set([]spaceassets.Asset{
		{Rel: "Plugs/atomdown-board.plug.js", Data: atomdownBoardPlug, Src: "plugs/atomdown-board/atomdown-board.plug.js"},
		{Rel: "Plugs/atomdown-inline.plug.js", Data: atomdownInlinePlug, Src: "plugs/atomdown-inline/atomdown-inline.plug.js"},
		{Rel: "Inline.md", Data: atomdownInlineLibrary, Src: "plugs/atomdown-inline/library/Atomdown Inline.md"},
	})
}

const usage = `Usage: iugum <up|container|agent|net|beads|beadview|wiki|observe|run|job|prepare-pr|skill>

  up           start wiki, observe, jobs/hooks/watch, code-server, browser, and ttyd in one process
  container    build or stop the iugum image (docker or podman)
  agent        scaffold and manage per-agent homes
  beads        work-graph slot (default: beads)
  beadview     beads viewer: React UI + JSON API at /api, legacy HTML pages at /legacy
  wiki         notes-server slot (default: SilverBullet)
  observe      metrics+logs store and graph UI (sqlite + uPlot)
  net          network policy: plan | apply [--dry-run] | show (iptables or nftables)
  run          start jobs, file watch, and optional HTTP POST /hooks/{name}
  job          list, add, remove, or run cron jobs (jobs.yaml)
  prepare-pr   write review files; do not push
  skill run    run a skill by name (prepare-pr)

  stage-wiki-assets <sb-src-dir>
               build step: copy this program's space assets into that tree's
               client_bundle/base_fs, so the next cargo build compiles them
               into the SilverBullet binary. Use scripts/build-wiki-blob.sh.

  check-wiki-assets [repo-dir]
               compare the plug bytes this binary serves against the plug
               sources in the tree. Exit 1 when one is behind. Silent about a
               tree it cannot find.

  iugum up [--wiki-port N] [--observe-port N] [--code-server-port N] [--browser-port N] [--ttyd-port N]
  iugum up [--no-code-server] [--no-browser] [--no-ttyd]
  iugum up --container [--image IMG] [--engine docker|podman|auto] [--name N] [--detach] [--dry-run]
  iugum container build [--with LIST] [--code-server 1|0] [--browser 1|0] [--tag T] [--engine E] [--dry-run]
  iugum container stop [--name N] [--engine E] [--dry-run]
  iugum agent init <name>
  iugum agent run --config FILE [--listen 127.0.0.1:3850]
  iugum agent up|down <name> [--engine E] [--dry-run]
  iugum agent status <name>
  iugum agent ls
  iugum agent tui|acp <name> [--dry-run]
  iugum agent checkpoint <name>
  iugum beads [bd args...]
  iugum beadview [--port N] [--hostname ADDR] [--dir DIR] [--read-only]
  iugum wiki [--port N] [--hostname ADDR] [--addons FILE] [space-dir]
  iugum check-wiki-assets [repo-dir]
  iugum observe [--port N] [--hostname ADDR]
  iugum net plan | apply [--dry-run] | show
  iugum job ls | add <name> [--every 1h|--spec SPEC] [--kind exec|http|session] [--prompt T] -- [cmd...]
  iugum job rm <name> | run <name>
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
	if len(args) > 1 && args[0] == "agent" && args[1] == "run" {
		if data, e := nativeAgentDataDir(args[2:]); e != nil {
			fmt.Fprintf(os.Stderr, "agent config: %v\n", e)
			return 1
		} else if data != "" {
			cfg.DataDir = data
		}
	}
	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iugum: %v\n", err)
		return 1
	}
	ctx := context.Background()

	switch args[0] {
	case "up":
		return runUp(ctx, a, cfg, args[1:])
	case "container":
		return runContainer(ctx, a, cfg, args[1:])
	case "agent":
		return runAgent(ctx, a, args[1:])
	case "run":
		return runRuntime(ctx, a, cfg)
	case "job":
		return runJob(ctx, a, args[1:])
	case "beads":
		if err := a.RunTracker(ctx, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			return 1
		}
		return 0
	case "beadview":
		port, host, dir, readOnly, code, ok := parseBeadViewArgs(args[1:])
		if !ok {
			return code
		}
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		extras := app.BeadViewExtras{ReadOnly: readOnly, UI: beadviewUI()}
		if err := a.ServeBeadViewWith(ctx, port, host, dir, extras); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			return 1
		}
		return 0
	case "net":
		return runNet(ctx, a, args[1:])
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
		wikiArgs, addons, err := extractWikiAddons(args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		port, host, space, code, ok := parseWikiArgs(wikiArgs)
		if !ok {
			return code
		}
		if addons != "" {
			return runWikiAddons(ctx, a, contract.WikiOpts{Port: port, Host: host, Space: space}, addons, os.Stderr)
		}
		if err := a.ServeWiki(ctx, contract.WikiOpts{Port: port, Host: host, Space: space}); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			if ee, ok := err.(interface{ ExitCode() int }); ok {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	case "stage-wiki-assets":
		return runStageWikiAssets(ctx, a, args[1:])
	case "check-wiki-assets":
		return runCheckWikiAssets(ctx, a, args[1:])
	case "observe":
		port, host, code, ok := parseObserveArgs(args[1:])
		if !ok {
			return code
		}
		ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := a.ServeObserve(ctx, port, host); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n%s", args[0], usage)
		return 1
	}
}

// runStageWikiAssets copies this program's space assets into the base_fs of a
// SilverBullet source tree, so the next cargo build compiles them into the
// SilverBullet binary. It is a build step, called by
// scripts/build-wiki-blob.sh between `npm run build` and `cargo build`.
//
// The staging lives in this program because the assets do: it holds the only
// copy, so a shell script cannot drift from it.
func runStageWikiAssets(ctx context.Context, a *app.App, args []string) int {
	if err := a.Check(ctx, "wiki", "stage"); err != nil {
		fmt.Fprintln(os.Stderr, app.DenyMessage(err))
		return 1
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "Usage: iugum stage-wiki-assets <silverbullet-source-dir>")
		return 2
	}
	written, err := spaceassets.Stage(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "stage-wiki-assets: %v\n", err)
		return 1
	}
	for _, rel := range written {
		fmt.Printf("staged %s\n", rel)
	}
	return 0
}

// runCheckWikiAssets answers one question: is what this binary serves the same
// as what the repository holds. It does the arithmetic, so a person does not
// have to compare two byte counts by eye.
//
// It reads and prints only. Exit 0 says every asset it could read is current,
// exit 1 says at least one is behind and names the fix. A tree it cannot find
// is exit 0 with one line saying so, because a binary with no sources beside it
// is not a fault.
func runCheckWikiAssets(ctx context.Context, a *app.App, args []string) int {
	return runCheckWikiAssetsIO(ctx, a, args, os.Stdout, os.Stderr)
}

func runCheckWikiAssetsIO(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if err := a.Check(ctx, "wiki", "check"); err != nil {
		fmt.Fprintln(stderr, app.DenyMessage(err))
		return 1
	}
	root := ""
	switch len(args) {
	case 0:
		root = spaceassets.SourceRoot()
	case 1:
		if strings.HasPrefix(args[0], "-") {
			fmt.Fprintln(stderr, "Usage: iugum check-wiki-assets [repo-dir]")
			return 2
		}
		root = args[0]
	default:
		fmt.Fprintln(stderr, "Usage: iugum check-wiki-assets [repo-dir]")
		return 2
	}
	if root == "" {
		fmt.Fprintf(stdout, "no iugum source tree above %s, so there is nothing to compare.\n", mustGetwd())
		fmt.Fprintf(stdout, "run this from a checkout, or set %s to one.\n", spaceassets.EnvSourceRoot)
		return 0
	}

	sts := spaceassets.Compare(root)
	fmt.Fprintf(stdout, "comparing the assets in this binary against %s\n\n", root)
	for _, st := range sts {
		switch {
		case !st.Found:
			fmt.Fprintf(stdout, "  %-34s %8d bytes  no source to compare\n", st.Rel, st.Carried)
		case st.Current:
			fmt.Fprintf(stdout, "  %-34s %8d bytes  current\n", st.Rel, st.Carried)
		case st.SizeOnly:
			fmt.Fprintf(stdout, "  %-34s %8d bytes  BEHIND: source is %d bytes too, and the contents differ\n", st.Rel, st.Carried, st.OnDisk)
		default:
			fmt.Fprintf(stdout, "  %-34s %8d bytes  BEHIND: source is %d bytes (%+d)\n", st.Rel, st.Carried, st.OnDisk, st.OnDisk-st.Carried)
		}
	}
	stale := spaceassets.Stale(sts)
	fmt.Fprintln(stdout)
	if len(stale) == 0 {
		fmt.Fprintln(stdout, "every asset this binary serves is the one in the tree.")
		return 0
	}
	fmt.Fprintf(stdout, "%d asset(s) are behind the tree. Rebuild: scripts/build-wiki-blob.sh\n", len(stale))
	return 1
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "the working directory"
	}
	return dir
}

func runRuntime(ctx context.Context, a *app.App, cfg config.File) int {
	if err := app.RegisterJobs(a, cfg.Jobs); err != nil {
		fmt.Fprintf(os.Stderr, "schedule: %v\n", err)
		return 1
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

func parseBeadViewArgs(args []string) (port int, host, dir string, readOnly bool, code int, ok bool) {
	port = 3849
	host = "127.0.0.1"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			fmt.Fprint(os.Stdout, usage)
			return 0, "", "", false, 0, false
		case a == "--read-only":
			readOnly = true
		case a == "--port" || a == "-p":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "beadview: --port requires a value")
				return 0, "", "", false, 2, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "beadview: invalid port %q\n", args[i])
				return 0, "", "", false, 2, false
			}
			port = n
		case strings.HasPrefix(a, "--port="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--port="))
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "beadview: invalid port %q\n", a)
				return 0, "", "", false, 2, false
			}
			port = n
		case a == "--hostname" || a == "-L":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "beadview: --hostname requires a value")
				return 0, "", "", false, 2, false
			}
			i++
			host = args[i]
		case strings.HasPrefix(a, "--hostname="):
			host = strings.TrimPrefix(a, "--hostname=")
			if host == "" {
				fmt.Fprintln(os.Stderr, "beadview: --hostname requires a value")
				return 0, "", "", false, 2, false
			}
		case a == "--dir" || a == "-C":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "beadview: --dir requires a value")
				return 0, "", "", false, 2, false
			}
			i++
			dir = args[i]
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
			if dir == "" {
				fmt.Fprintln(os.Stderr, "beadview: --dir requires a value")
				return 0, "", "", false, 2, false
			}
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "beadview: unknown flag %s\n", a)
			return 0, "", "", false, 2, false
		default:
			fmt.Fprintf(os.Stderr, "beadview: extra argument %s\n", a)
			return 0, "", "", false, 2, false
		}
	}
	return port, host, dir, readOnly, 0, true
}

func parseObserveArgs(args []string) (port int, host string, code int, ok bool) {
	port = 3848
	host = "127.0.0.1"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help" || a == "-h":
			fmt.Fprint(os.Stdout, usage)
			return 0, "", 0, false
		case a == "--port" || a == "-p":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "observe: --port requires a value")
				return 0, "", 2, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "observe: invalid port %q\n", args[i])
				return 0, "", 2, false
			}
			port = n
		case strings.HasPrefix(a, "--port="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--port="))
			if err != nil || n <= 0 || n > 65535 {
				fmt.Fprintf(os.Stderr, "observe: invalid port %q\n", a)
				return 0, "", 2, false
			}
			port = n
		case a == "--hostname" || a == "-L":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "observe: --hostname requires a value")
				return 0, "", 2, false
			}
			i++
			host = args[i]
		case strings.HasPrefix(a, "--hostname="):
			host = strings.TrimPrefix(a, "--hostname=")
			if host == "" {
				fmt.Fprintln(os.Stderr, "observe: --hostname requires a value")
				return 0, "", 2, false
			}
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "observe: unknown flag %s\n", a)
			return 0, "", 2, false
		default:
			fmt.Fprintf(os.Stderr, "observe: extra argument %s\n", a)
			return 0, "", 2, false
		}
	}
	return port, host, 0, true
}
