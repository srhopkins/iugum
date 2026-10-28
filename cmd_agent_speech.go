package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/speechproxy"
)

func runAgentSpeechRelay(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent speech-relay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:8766", "literal loopback listener")
	upstream := fs.String("upstream", "", "required fixed HTTP(S) upstream root URL")
	timeout := fs.Duration("timeout", 60*time.Second, "upstream deadline, at most60seconds")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected speech relay arguments")
		return 2
	}
	if err := speechproxy.ValidateListen(*listen); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *upstream == "" {
		fmt.Fprintln(stderr, "explicit --upstream is required; no destination is selected by default")
		return 2
	}
	if a == nil || a.Gate == nil {
		fmt.Fprintln(stderr, "speech relay application policy required")
		return 2
	}
	relay, err := speechproxy.New(speechproxy.Config{Upstream: *upstream, Timeout: *timeout, Check: a.Check})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer relay.Close()
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(stderr, "speech relay startup cancelled")
		return 1
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(stderr, "speech relay listener unavailable")
		return 1
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	server := &http.Server{BaseContext: func(net.Listener) context.Context { return ctx }, Handler: relay, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 65 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 32 * 1024}
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Fprintf(stdout, "Chat speech relay listening at http://%s (explicit fixed upstream; no automatic requests)\n", listener.Addr())
	if err = server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, "speech relay server stopped unexpectedly")
		return 1
	}
	return 0
}
