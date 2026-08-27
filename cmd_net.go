package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/srhopkins/iugum/app"
)

const netUsage = `Usage: iugum net <plan|apply [--dry-run]|show>

  plan             print the rendered rules; no change
  apply            write the rules (linux + CAP_NET_ADMIN); --dry-run prints only
  show             print the live ruleset (iptables -S / nft list ruleset)
`

// runNet is the CLI for the network-policy slot.
func runNet(ctx context.Context, a *app.App, args []string) int {
	return runNetIO(ctx, a, args, os.Stdout, os.Stderr)
}

func runNetIO(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, netUsage)
		return 0
	}
	switch args[0] {
	case "plan":
		lines, err := a.PlanNet(ctx)
		if err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		printLines(stdout, lines)
		return 0
	case "apply":
		dry := false
		for _, f := range args[1:] {
			switch f {
			case "--dry-run", "-n":
				dry = true
			default:
				fmt.Fprintf(stderr, "net apply: unknown flag %s\n", f)
				return 2
			}
		}
		if dry {
			lines, err := a.DryRunNet(ctx)
			if err != nil {
				fmt.Fprintln(stderr, app.DenyMessage(err))
				return 1
			}
			printLines(stdout, lines)
			return 0
		}
		if err := a.ApplyNet(ctx); err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		if a.Net != nil {
			fmt.Fprintf(stdout, "iugum net: applied %d rule(s) via %s\n", len(a.NetRules.Rules), a.Net.Name())
		}
		return 0
	case "show":
		out, err := a.ShowNet(ctx)
		if err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		fmt.Fprint(stdout, out)
		return 0
	default:
		fmt.Fprintf(stderr, "net: unknown subcommand %s\n\n%s", args[0], netUsage)
		return 2
	}
}

// printLines writes each rendered item. nftables gives one multi-line item.
func printLines(w io.Writer, lines []string) {
	for _, l := range lines {
		if strings.HasSuffix(l, "\n") {
			fmt.Fprint(w, l)
		} else {
			fmt.Fprintln(w, l)
		}
	}
}
