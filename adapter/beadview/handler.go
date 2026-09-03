package beadview

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// NewHandler builds the beadview HTTP handler. f supplies data; production
// callers pass NewExecFetcher(exe, dir), tests pass a fake Fetcher.
func NewHandler(f Fetcher) http.Handler {
	mux := http.NewServeMux()
	h := &server{f: f}
	mux.HandleFunc("GET /{$}", h.handleList)
	mux.HandleFunc("GET /bead/{id}", h.handleDetail)
	mux.HandleFunc("GET /graph", h.handleGraph)
	return mux
}

type server struct {
	f Fetcher
}

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
