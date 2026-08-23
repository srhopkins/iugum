package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	_ "github.com/srhopkins/iugum/defaults"
	"github.com/srhopkins/iugum/embedbin"
)

//go:embed silverbullet/silverbullet
var silverbulletBin []byte

func init() {
	embedbin.Set(silverbulletBin)
}

const usage = `Usage: iugum <beads|wiki>

  beads    work-graph slot (default: beads)
  wiki     notes-server slot (default: SilverBullet)

  iugum beads [bd args...]
  iugum wiki [--port N] [--hostname ADDR] [space-dir]

  wiki defaults: port 3000, space ./wiki, listen 127.0.0.1
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
	case "beads":
		if err := a.RunTracker(ctx, args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, app.DenyMessage(err))
			return 1
		}
		return 0
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
