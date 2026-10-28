// Package beadview embeds Iugum-owned additions to the pinned Beads viewer.
package beadview

import (
	"embed"
	fork "github.com/steveyegge/beads/beadview"
	"io/fs"
)

//go:embed assets
var assets embed.FS

func UI() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err == nil {
		if _, err := fs.Stat(sub, "index.html"); err == nil {
			return sub
		}
	}
	return fork.UI()
}
