package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/providerstartup"
)

func runAgentSessionLifecycle(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent sessions "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	config := fs.String("config", "", "protected lifecycle manifest")
	from := fs.String("from", "", "reviewed non-secret manifest used for initial setup")
	if e := fs.Parse(args[1:]); e != nil {
		return 2
	}
	if *config == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "Usage: iugum agent sessions setup|start --config FILE [--from INITIAL-MANIFEST]")
		return 2
	}
	// Starting the same session service retains its existing application gate.
	if e := a.Check(ctx, "agent/sessions", "serve"); e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	source := *config
	if args[0] == "setup" && *from != "" {
		source = *from
	}
	if args[0] == "start" && *from != "" {
		fmt.Fprintln(stderr, "--from is only valid for setup")
		return 2
	}
	settings, e := providerstartup.Load(source)
	if e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	if args[0] == "setup" {
		binary, e := os.Executable()
		if e == nil {
			e = providerstartup.Setup(settings, *config, binary)
		}
		if e != nil {
			fmt.Fprintln(stderr, e)
			return 1
		}
		fmt.Fprintln(stdout, "Provider startup configured; saved credential reused or created privately for managed mode")
		return 0
	}
	if args[0] == "wait-ready" {
		if e = providerstartup.WaitReady(ctx, settings); e != nil {
			fmt.Fprintln(stderr, e)
			return 1
		}
		return 0
	}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e = providerstartup.Run(ctx, settings); e != nil {
		fmt.Fprintln(stderr, e)
		return 1
	}
	return 0
}
