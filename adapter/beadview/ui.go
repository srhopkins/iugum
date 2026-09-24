package beadview

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// placeholderHTML is served at / when no UI build was given (Options.UI is
// nil, e.g. in handler tests or a custom embedder).
const placeholderHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8">
<title>iugum beadview</title></head><body>
<p>The beadview UI build is not embedded in this binary.
Build it with <code>npm run build</code> in <code>web/beadview/</code>, then rebuild iugum.</p>
<p>The server-rendered pages still work: <a href="/legacy/">/legacy/</a>.
The JSON API is at <a href="/api/beads">/api/beads</a>.</p>
</body></html>`

// spaHandler serves the React build from ui (the contents of dist/, with
// index.html at its root). Rules:
//   - an existing file is served as-is (hashed files under /assets/ get a
//     long cache; everything else, including index.html, is no-cache);
//   - a missing file under /assets/ is a real 404, so a stale bundle fails
//     loudly instead of receiving HTML with a JS MIME expectation;
//   - any other unknown path returns index.html (client-side routes).
func spaHandler(ui fs.FS) http.Handler {
	if ui == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(placeholderHTML))
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if fi, err := fs.Stat(ui, name); err == nil && !fi.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFileFS(w, r, ui, name)
				return
			}
			if strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		serveIndex(w, r, ui)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, ui fs.FS) {
	b, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		http.Error(w, "beadview UI build has no index.html", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
}
