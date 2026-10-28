package beadsadapt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	activityui "github.com/srhopkins/iugum/web/beadview"
	"github.com/steveyegge/beads/beadview"
)

// WebOptions configure the fork's `bd web` viewer when iugum runs it.
type WebOptions struct {
	// Exe is the binary that answers bead queries (iugum itself).
	Exe string
	// Port 0 means 3849. Host "" means 127.0.0.1.
	Port         int
	Host         string
	Dir          string
	Projects     string
	ViewerConfig string
	ReadOnly     bool
	// Authorize is asked before each write request.
	Authorize func(ctx context.Context, action string) error
}

// ServeWeb runs the Beads fork's web command with iugum's settings. Queries
// run as "<exe> beads -C <dir> ...", so the memory hook stays in effect.
func ServeWeb(ctx context.Context, o WebOptions) error {
	if o.Port == 0 {
		o.Port = 3849
	}
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}

	if o.Port < 1 || o.Port > 65535 {
		return fmt.Errorf("beadview: invalid port %d", o.Port)
	}
	opts := beadview.Options{ReadOnly: o.ReadOnly, UI: activityui.UI(), AuthorizeWrite: o.Authorize}
	if o.ViewerConfig != "" {
		cfg, err := beadview.LoadViewerConfig(o.ViewerConfig)
		if err != nil {
			return err
		}
		opts.ViewerConfig = cfg
	}
	var projects []beadview.Project
	var activityProjects []activityProject
	multi := o.Projects != ""
	if multi {
		if o.Dir != "" {
			return errors.New("beadview: --projects and --dir cannot be used together")
		}
		cfgs, err := beadview.LoadProjects(o.Projects)
		if err != nil {
			return err
		}
		for _, c := range cfgs {
			projects = append(projects, beadview.Project{Name: c.Name, F: beadview.NewExecFetcherEntry(o.Exe, []string{"beads"}, c.Path)})
			activityProjects = append(activityProjects, activityProject{c.Name, c.Path})
		}
	} else {
		dir := o.Dir
		if dir == "" {
			var err error
			dir, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		dir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		if fi, err := os.Stat(filepath.Join(dir, ".beads")); err != nil || !fi.IsDir() {
			return fmt.Errorf("beadview: no .beads/ directory in %s", dir)
		}
		projects = []beadview.Project{{Name: filepath.Base(dir), F: beadview.NewExecFetcherEntry(o.Exe, []string{"beads"}, dir)}}
		activityProjects = []activityProject{{filepath.Base(dir), dir}}
	}
	var handler http.Handler
	if multi {
		handler = beadview.NewMultiHandler(projects, opts)
	} else {
		handler = beadview.NewHandler(projects[0].F, opts)
	}
	run := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, o.Exe, append([]string{"beads", "-C", dir}, args...)...)
		return cmd.Output()
	}
	handler = activityHandler(handler, activityProjects, run, time.Now)
	address := net.JoinHostPort(o.Host, fmt.Sprint(o.Port))
	srv := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	fmt.Fprintf(os.Stdout, "iugum beadview: http://%s\n", address)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
