package app

import (
	"context"
	"os"

	"github.com/srhopkins/iugum/adapter/tracker/beadsadapt"
)

// osExecutable resolves iugum's own binary path. A package var, not a bare
// os.Executable() call, so tests can point it at a real built iugum binary:
// the test process itself is a go-test binary with no "beads" subcommand.
var osExecutable = os.Executable

// ServeBeadView starts the beads viewer. The viewer is the `bd web` command
// of the Beads fork (github.com/srhopkins/beads-iugum, package beadview). It
// answers every query by re-running iugum's own binary as
// "iugum beads -C <dir> ...". Port 0 means 3849. dir is the target beads repo
// (must contain .beads); empty means the process cwd.
func (a *App) ServeBeadView(ctx context.Context, port int, host, dir string) error {
	return a.ServeBeadViewWith(ctx, port, host, dir, BeadViewExtras{})
}

// BeadViewExtras are the beadview options beyond port/host/dir.
type BeadViewExtras struct {
	// ReadOnly disables the JSON API's write endpoints (--read-only).
	ReadOnly bool
	// ProjectsFile is the --projects YAML file. When set, dir is ignored and
	// the viewer serves every project in it.
	ProjectsFile string
}

// ServeBeadViewWith runs the fork's web command. The Casbin action
// beadview/serve gates start-up. beadview/write gates each write request
// (allowed by the default allow-all policy; deny it to lock writes).
func (a *App) ServeBeadViewWith(ctx context.Context, port int, host, dir string, extras BeadViewExtras) error {
	if err := a.Check(ctx, "beadview", "serve"); err != nil {
		return err
	}
	exe, err := osExecutable()
	if err != nil {
		return err
	}
	return beadsadapt.ServeWeb(ctx, beadsadapt.WebOptions{
		Exe:      exe,
		Port:     port,
		Host:     host,
		Dir:      dir,
		Projects: extras.ProjectsFile,
		ReadOnly: extras.ReadOnly,
		Authorize: func(ctx context.Context, _ string) error {
			return a.Check(ctx, "beadview", "write")
		},
	})
}
