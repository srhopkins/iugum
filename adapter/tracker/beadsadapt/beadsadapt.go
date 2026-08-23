package beadsadapt

import (
	"context"
	"os"

	bdcmd "github.com/steveyegge/beads/cmd/bd"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterTracker("beads", func(map[string]string) (contract.Tracker, error) {
		return Tracker{}, nil
	})
}

// Tracker runs in-process beads (bd cobra).
type Tracker struct{}

func (Tracker) Name() string { return "beads" }

func (Tracker) Run(_ context.Context, args []string) error {
	if os.Getenv("BD_NAME") == "" {
		_ = os.Setenv("BD_NAME", "iugum")
	}
	os.Args = append([]string{os.Args[0]}, args...)
	bdcmd.Execute()
	return nil
}
