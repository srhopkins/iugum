package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	return a.ServeBeadViewWith(ctx, port, host, dir, BeadViewExtras{})
}

// BeadViewExtras are the beadview options beyond port/host/dir.
type BeadViewExtras struct {
	// ReadOnly disables the JSON API's write endpoints (--read-only).
	ReadOnly bool
	// UI is the embedded React build (index.html at its root). Nil serves
	// a placeholder page at /.
	UI fs.FS
	// ProjectsFile is the --projects YAML file. When set, dir is ignored and
	// the viewer serves every project in it.
	ProjectsFile string
}

// ServeBeadViewWith is ServeBeadView plus the read-only flag and the UI
// build. Writes are gated per request by the Casbin action beadview/write
// (allowed by the default allow-all policy; deny it to lock writes).
func (a *App) ServeBeadViewWith(ctx context.Context, port int, host, dir string, extras BeadViewExtras) error {
	if err := a.Check(ctx, "beadview", "serve"); err != nil {
		return err
	}
	if port == 0 {
		port = 3849
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if extras.ProjectsFile != "" {
		return a.serveBeadViewProjects(ctx, port, host, extras)
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

	handler := beadview.NewHandler(beadview.NewExecFetcher(exe, abs), beadview.Options{
		ReadOnly: extras.ReadOnly,
		UI:       extras.UI,
		AuthorizeWrite: func(ctx context.Context, action string) error {
			return a.Check(ctx, "beadview", "write")
		},
	})
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: handler}

	fmt.Fprintf(os.Stdout, "iugum beadview: http://%s\n", addr)
	fmt.Fprintf(os.Stdout, "iugum beadview data: %s\n", abs)
	if extras.ReadOnly {
		fmt.Fprintln(os.Stdout, "iugum beadview: read-only (write endpoints return 405)")
	}

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

// serveBeadViewProjects is the --projects mode: one viewer over every
// tracker in the file. Each project gets its own exec fetcher, so each one
// keeps its own child-process lock.
func (a *App) serveBeadViewProjects(ctx context.Context, port int, host string, extras BeadViewExtras) error {
	cfgs, err := beadview.LoadProjects(extras.ProjectsFile)
	if err != nil {
		return err
	}
	exe, err := osExecutable()
	if err != nil {
		return fmt.Errorf("beadview: resolving own executable: %w", err)
	}
	projects := make([]beadview.Project, 0, len(cfgs))
	for _, c := range cfgs {
		projects = append(projects, beadview.Project{Name: c.Name, F: beadview.NewExecFetcher(exe, c.Path)})
	}
	handler := beadview.NewMultiHandler(projects, beadview.Options{
		ReadOnly: extras.ReadOnly,
		UI:       extras.UI,
		AuthorizeWrite: func(ctx context.Context, action string) error {
			return a.Check(ctx, "beadview", "write")
		},
	})
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: handler}
	fmt.Fprintf(os.Stdout, "iugum beadview: http://%s\n", addr)
	for _, c := range cfgs {
		fmt.Fprintf(os.Stdout, "iugum beadview project %s: %s\n", c.Name, c.Path)
	}
	if extras.ReadOnly {
		fmt.Fprintln(os.Stdout, "iugum beadview: read-only (write endpoints return 405)")
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
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
