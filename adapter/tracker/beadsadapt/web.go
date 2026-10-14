package beadsadapt

import (
	"context"
	"strconv"

	bdcmd "github.com/steveyegge/beads/cmd/bd"
)

// WebOptions configure the fork's `bd web` viewer when iugum runs it.
type WebOptions struct {
	// Exe is the binary that answers bead queries (iugum itself).
	Exe string
	// Port 0 means 3849. Host "" means 127.0.0.1.
	Port     int
	Host     string
	Dir      string
	Projects string
	ReadOnly bool
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
	args := []string{"--port", strconv.Itoa(o.Port), "--hostname", o.Host}
	if o.Projects != "" {
		args = append(args, "--projects", o.Projects)
	} else if o.Dir != "" {
		args = append(args, "--dir", o.Dir)
	}
	if o.ReadOnly {
		args = append(args, "--read-only")
	}
	return bdcmd.RunWeb(ctx, bdcmd.WebHooks{
		Name:           "iugum beadview",
		Exe:            o.Exe,
		Entry:          []string{"beads"},
		AuthorizeWrite: o.Authorize,
	}, args)
}
