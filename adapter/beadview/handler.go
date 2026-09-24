package beadview

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
)

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// Options configures NewHandler. The zero value is a writable server with
// no UI build (the placeholder page at /), logging to stderr.
type Options struct {
	// ReadOnly disables every write endpoint with 405 (--read-only).
	ReadOnly bool
	// UI is the React build: an fs.FS whose root holds index.html and
	// assets/. Nil serves a placeholder page at /.
	UI fs.FS
	// AuthorizeWrite, if set, is asked before each write. action is one of
	// create, update, close, reopen, comment. A non-nil error answers 403.
	AuthorizeWrite func(ctx context.Context, action string) error
	// Log receives one line per write attempt. Nil means stderr.
	Log *log.Logger
}

// NewHandler builds the beadview HTTP handler. f supplies data; production
// callers pass NewExecFetcher(exe, dir), tests pass a fake Fetcher.
//
// Routes: /api/* is the JSON API (api.go), also mounted at /api/bd/* so
// ayo-style clients work unchanged; /legacy/* is the server-rendered
// html/template viewer; /bead/{id}, /tree and /graph redirect (old
// bookmarks); everything else is the React build with SPA fallback.
func NewHandler(f Fetcher, opts Options) http.Handler {
	if opts.Log == nil {
		opts.Log = log.New(os.Stderr, "beadview: ", log.LstdFlags)
	}
	mux := http.NewServeMux()
	h := &server{f: f, opts: opts}

	h.registerAPI(mux, "/api")
	h.registerAPI(mux, "/api/bd")

	mux.HandleFunc("GET /legacy/{$}", h.handleList)
	mux.HandleFunc("GET /legacy/bead/{id}", h.handleDetail)
	mux.HandleFunc("GET /legacy/tree", h.handleTree)
	mux.HandleFunc("GET /legacy/graph", h.handleGraph)
	mux.Handle("GET /legacy", http.RedirectHandler("/legacy/", http.StatusMovedPermanently))

	// Old URLs. /bead/{id} opens the new UI with that bead selected (the
	// UI reads ?bead=). /graph and /tree keep their old pages under
	// /legacy: bd's D3 graph and the parent-nested tree have no exact
	// equivalent in the new UI.
	mux.HandleFunc("GET /bead/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/?bead="+url.QueryEscape(r.PathValue("id")), http.StatusFound)
	})
	mux.HandleFunc("GET /graph", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/legacy/graph", http.StatusFound)
	})
	mux.HandleFunc("GET /tree", func(w http.ResponseWriter, r *http.Request) {
		target := "/legacy/tree"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusFound)
	})

	mux.Handle("/", spaHandler(opts.UI))
	return mux
}

type server struct {
	f    Fetcher
	opts Options
}

func (s *server) logf(format string, args ...any) { s.opts.Log.Printf(format, args...) }

type listPage struct {
	Title         string
	Active        string
	Dir           string
	Beads         []Bead
	Total         int
	Filter        Filter
	StatusOptions []string
	TypeOptions   []string
	SortOptions   []string
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	beads, err := s.f.FetchBeads(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}
	q := r.URL.Query()
	filter := Filter{
		Status: q.Get("status"),
		Type:   q.Get("type"),
		Search: q.Get("q"),
		Sort:   q.Get("sort"),
	}
	filtered := ApplyFilter(beads, filter)
	page := listPage{
		Title:         "Tickets",
		Active:        "list",
		Dir:           s.f.Dir(),
		Beads:         filtered,
		Total:         len(beads),
		Filter:        filter,
		StatusOptions: Statuses,
		TypeOptions:   Types,
		SortOptions:   []string{"updated", "created", "priority", "status", "id", "title"},
	}
	s.render(w, "list-content", page.Title, page.Active, page)
}

type treePage struct {
	Title  string
	Active string
	Dir    string
	Roots  []*TreeNode
	Total  int
	Filter Filter
}

// handleTree groups beads by Bead.Parent (epic hierarchy) into an indented,
// expand/collapse tree, unlike handleList's flat table. See the beadview
// README "What this does not do" for why the table stays flat.
func (s *server) handleTree(w http.ResponseWriter, r *http.Request) {
	beads, err := s.f.FetchBeads(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}
	filter := Filter{Search: r.URL.Query().Get("q")}
	roots := PruneTree(BuildTree(beads), filter.Search)
	page := treePage{
		Title:  "Tree",
		Active: "tree",
		Dir:    s.f.Dir(),
		Roots:  roots,
		Total:  len(beads),
		Filter: filter,
	}
	s.render(w, "tree-content", page.Title, page.Active, page)
}

type detailPage struct {
	Title    string
	Active   string
	Dir      string
	Bead     *Bead
	Comments []Comment
}

func (s *server) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	bead, err := s.f.FetchBead(r.Context(), id)
	if err != nil {
		s.renderError(w, err)
		return
	}
	comments, err := s.f.FetchComments(r.Context(), id)
	if err != nil {
		// Comments are supplementary; a fetch failure should not hide the bead.
		comments = nil
	}
	page := detailPage{
		Title:    bead.ID,
		Active:   "detail",
		Dir:      s.f.Dir(),
		Bead:     bead,
		Comments: comments,
	}
	s.render(w, "detail-content", page.Title, page.Active, page)
}

// handleGraph serves bd's own self-contained D3 dependency graph verbatim.
func (s *server) handleGraph(w http.ResponseWriter, r *http.Request) {
	html, err := s.f.FetchGraphHTML(r.Context())
	if err != nil {
		s.renderError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// layoutData is what layout.html itself renders: chrome plus a pre-rendered
// content fragment. Body is trusted HTML produced by our own templates
// above, never user input, so template.HTML here does not reopen escaping.
type layoutData struct {
	Title  string
	Active string
	Dir    string
	Body   template.HTML
}

func (s *server) render(w http.ResponseWriter, contentName, title, active string, data any) {
	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, contentName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := layoutData{Title: title, Active: active, Dir: s.f.Dir(), Body: template.HTML(body.String())}
	if err := tmpl.ExecuteTemplate(w, "layout", page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) renderError(w http.ResponseWriter, err error) {
	var body bytes.Buffer
	_ = tmpl.ExecuteTemplate(&body, "error-content", struct{ Message string }{err.Error()})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	page := layoutData{Title: "Error", Active: "", Dir: s.f.Dir(), Body: template.HTML(body.String())}
	_ = tmpl.ExecuteTemplate(w, "layout", page)
}
