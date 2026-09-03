package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/srhopkins/iugum/adapter/beadview"
)

// osExecutable resolves iugum's own binary path. A package var, not a bare
// os.Executable() call, so tests can point it at a real built iugum binary:
// the test process itself is a go-test binary with no "beads" subcommand,
// so the real function only resolves correctly when iugum is actually
// running as iugum. See adapter/beadview/README.md "Data path".
var osExecutable = os.Executable

// ServeBeadView starts the read-mostly beads viewer: a table of tickets plus
// bd's own interactive dependency graph. Port 0 means 3849. dir is the
// target beads repo (must contain .beads); empty means the process cwd.
//
// The data path re-execs iugum's own binary as "iugum beads -C <dir> ...".
// It never shells out to an external bd. See adapter/beadview/README.md
// "Data path" for why a subprocess and not a direct in-process call.
func (a *App) ServeBeadView(ctx context.Context, port int, host, dir string) error {
	if err := a.Check(ctx, "beadview", "serve"); err != nil {
		return err
	}
	if port == 0 {
		port = 3849
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(abs, ".beads")); err != nil || !fi.IsDir() {
		return fmt.Errorf("beadview: no .beads/ directory in %s (pass a beads repo with --dir)", abs)
	}
	exe, err := osExecutable()
	if err != nil {
		return fmt.Errorf("beadview: resolving own executable: %w", err)
	}

	handler := beadview.NewHandler(beadview.NewExecFetcher(exe, abs))
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: handler}

	fmt.Fprintf(os.Stdout, "iugum beadview: http://%s\n", addr)
	fmt.Fprintf(os.Stdout, "iugum beadview data: %s\n", abs)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
