package main

import (
	"embed"
	"io/fs"
)

// beadviewDist is the React build for `iugum beadview` (web/beadview/dist:
// index.html plus assets/). The build is committed, like web/observe/dist.
// It lives in package main, not in web/beadview/, because web/beadview/ is
// the Node project and has no Go files. See adapter/beadview/README.md.
//
//go:embed all:web/beadview/dist
var beadviewDist embed.FS

// beadviewUI returns the build with dist/ stripped, so index.html is at
// its root. A broken embed returns nil, and beadview serves a placeholder.
func beadviewUI() fs.FS {
	sub, err := fs.Sub(beadviewDist, "web/beadview/dist")
	if err != nil {
		return nil
	}
	return sub
}
