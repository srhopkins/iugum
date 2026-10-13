package beadview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// APIBead is the bead shape the React UI reads. It follows the ayo viewer's
// normalizeViewerBead contract: `type` (not issue_type), priority as the
// string "p0".."p4", and depends_on/dependencies flattened to ID strings.
// The extra fields (issue_type, blocked_by, blocks, dependency_edges) are
// additive; the ayo code ignores them.
type APIBead struct {
	// Project is the name of the tracker this bead lives in. Bead IDs are
	// only unique inside one project, so (project, id) is the key.
	Project     string   `json:"project"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Notes       string   `json:"notes"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority,omitempty"`
	Type        string   `json:"type"`
	IssueType   string   `json:"issue_type"`
	Assignee    string   `json:"assignee"`
	Owner       string   `json:"owner"`
	CreatedBy   string   `json:"created_by"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	StartedAt   string   `json:"started_at"`
	ClosedAt    string   `json:"closed_at"`
	CloseReason string   `json:"close_reason"`
	Labels      []string `json:"labels"`
	Parent      string   `json:"parent"`
	// DependsOn and Dependencies are the same list: every depends_on_id
	// of this bead, any edge type. Both names exist because the ayo UI
	// reads either.
	DependsOn    []string `json:"depends_on"`
	Dependencies []string `json:"dependencies"`
	// BlockedBy is the subset of DependsOn joined by a "blocks" edge.
	BlockedBy []string `json:"blocked_by"`
	// Blocks is the reverse: IDs of beads that have a "blocks" edge onto
	// this one. Computed across the whole bead list.
	Blocks          []string     `json:"blocks"`
	DependencyEdges []Dependency `json:"dependency_edges"`
	DependencyCnt   int          `json:"dependency_count"`
	DependentCnt    int          `json:"dependent_count"`
	CommentCnt      int          `json:"comment_count"`
}

// ToAPI maps one bd bead to the UI shape. blocks is the reverse "blocks"
// index for this bead (may be nil).
func ToAPI(b Bead, blocks []string) APIBead {
	deps := make([]string, 0, len(b.Dependencies))
	for _, d := range b.Dependencies {
		if d.DependsOnID != "" {
			deps = append(deps, d.DependsOnID)
		}
	}
	edges := b.Dependencies
	if edges == nil {
		edges = []Dependency{}
	}
	return APIBead{
		ID:              b.ID,
		Title:           b.Title,
		Description:     b.Description,
		Notes:           b.Notes,
		Status:          orDefault(b.Status, "open"),
		Priority:        b.PriorityLabel(),
		Type:            b.IssueType,
		IssueType:       b.IssueType,
		Assignee:        b.Assignee,
		Owner:           b.Owner,
		CreatedBy:       b.CreatedBy,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
		StartedAt:       b.StartedAt,
		ClosedAt:        b.ClosedAt,
		CloseReason:     b.CloseReason,
		Labels:          nonNil(b.Labels),
		Parent:          b.Parent,
		DependsOn:       deps,
		Dependencies:    deps,
		BlockedBy:       nonNil(b.Blocks()),
		Blocks:          nonNil(blocks),
		DependencyEdges: edges,
		DependencyCnt:   b.DependencyCnt,
		DependentCnt:    b.DependentCnt,
		CommentCnt:      b.CommentCnt,
	}
}

// ToAPIList maps a whole list and fills each bead's reverse Blocks index.
func ToAPIList(beads []Bead) []APIBead {
	rev := reverseBlocks(beads)
	out := make([]APIBead, 0, len(beads))
	for _, b := range beads {
		out = append(out, ToAPI(b, rev[b.ID]))
	}
	return out
}

func reverseBlocks(beads []Bead) map[string][]string {
	rev := map[string][]string{}
	for _, b := range beads {
		for _, id := range b.Blocks() {
			rev[id] = append(rev[id], b.ID)
		}
	}
	return rev
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// idPattern is what a bead ID may look like on the API. It never starts
// with "-", so an ID can never be read as a bd flag. bd IDs look like
// "iugum-puu" or "bd-a1b2.3".
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validID(id string) bool { return idPattern.MatchString(id) }

// flexString accepts a JSON string or number ("p1", "1", 1) so priority
// works whether the UI sends the ayo "p1" form or a bare integer.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("priority must be a string or number")
	}
	*f = flexString(n.String())
	return nil
}

// priorityToBD converts "p1"/"P1"/"1" to bd's "1". It rejects anything
// outside 0..4 so the handler can answer 400 instead of a bd error.
func priorityToBD(p string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(p))
	s = strings.TrimPrefix(s, "p")
	if len(s) == 1 && s[0] >= '0' && s[0] <= '4' {
		return s, nil
	}
	return "", fmt.Errorf("priority %q must be p0..p4", p)
}

// registerAPI mounts the JSON API on mux under prefix ("/api"). Paths and
// shapes mirror the ayo viewer's /api/bd BFF so its React code runs as-is.
func (s *server) registerAPI(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/beads", s.apiListBeads)
	mux.HandleFunc("GET "+prefix+"/bead/{id}", s.apiGetBead)
	mux.HandleFunc("GET "+prefix+"/bead/{id}/comments", s.apiListComments)
	mux.HandleFunc("GET "+prefix+"/mermaid", s.apiMermaid)
	mux.HandleFunc("GET "+prefix+"/config", s.apiConfig)
	mux.HandleFunc("GET "+prefix+"/projects", s.apiProjects)

	// Project-qualified forms: /api/bead/{project}/{id}. The plain /api/bead/{id}
	// forms stay for the single-project UI, which sends ?project= instead.
	mux.HandleFunc("GET "+prefix+"/bead/{project}/{id}", s.apiGetBead)
	mux.HandleFunc("GET "+prefix+"/bead/{project}/{id}/comments", s.apiListComments)
	mux.HandleFunc("PATCH "+prefix+"/bead/{project}/{id}", s.write("update", s.apiUpdateBead))
	mux.HandleFunc("POST "+prefix+"/bead/{project}/{id}/close", s.write("close", s.apiCloseBead))
	mux.HandleFunc("POST "+prefix+"/bead/{project}/{id}/reopen", s.write("reopen", s.apiReopenBead))
	mux.HandleFunc("POST "+prefix+"/bead/{project}/{id}/comments", s.write("comment", s.apiAddComment))

	mux.HandleFunc("POST "+prefix+"/bead", s.write("create", s.apiCreateBead))
	mux.HandleFunc("PATCH "+prefix+"/bead/{id}", s.write("update", s.apiUpdateBead))
	mux.HandleFunc("POST "+prefix+"/bead/{id}/close", s.write("close", s.apiCloseBead))
	mux.HandleFunc("POST "+prefix+"/bead/{id}/reopen", s.write("reopen", s.apiReopenBead))
	mux.HandleFunc("POST "+prefix+"/bead/{id}/comments", s.write("comment", s.apiAddComment))

	// Anything else under the prefix is a JSON 404, never the SPA page.
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such API endpoint: "+r.Method+" "+r.URL.Path)
	})
}

func (s *server) apiListBeads(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("project")
	var sel []Project
	if s.multi && name != "" {
		p, ok := s.pick(w, r, "")
		if !ok {
			return
		}
		sel = []Project{*p}
	} else if s.multi {
		sel = s.projects
	} else {
		sel = s.projects[:1]
	}
	out, failed := s.listProjects(r.Context(), sel)
	if len(failed) == len(sel) {
		writeUpstreamError(w, failed[0])
		return
	}
	for _, e := range failed {
		w.Header().Add("X-Beadview-Project-Errors", strings.ReplaceAll(e.Error(), "\n", " "))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) apiGetBead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, ok := s.pick(w, r, "")
	if !ok {
		return
	}
	b, err := p.F.FetchBead(r.Context(), id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.withReverse(r.Context(), p, *b))
}

// reread fills in what a write's own result leaves out. The JSON that bd
// update, close and reopen print has no dependency edges and no parent, so
// a response built from it alone shows depends_on as empty. The write's
// result stays the source for every field it does carry; the edges, the
// parent and the counters come from a fresh `bd show`. If that read fails,
// the write's result is returned as it is.
func (s *server) reread(ctx context.Context, p *Project, b *Bead) APIBead {
	out := *b
	if fresh, err := p.F.FetchBead(ctx, b.ID); err == nil && fresh != nil {
		if len(out.Dependencies) == 0 {
			out.Dependencies = fresh.Dependencies
			out.DependencyCnt = fresh.DependencyCnt
		}
		if out.Parent == "" {
			out.Parent = fresh.Parent
		}
		if out.DependentCnt == 0 {
			out.DependentCnt = fresh.DependentCnt
		}
		if out.CommentCnt == 0 {
			out.CommentCnt = fresh.CommentCnt
		}
	}
	return s.withReverse(ctx, p, out)
}

// withReverse fills Blocks for a single bead. It needs the full list; if
// that read fails, the bead is still returned with an empty Blocks.
func (s *server) withReverse(ctx context.Context, p *Project, b Bead) APIBead {
	var rev []string
	if all, err := p.F.FetchBeads(ctx); err == nil {
		rev = reverseBlocks(all)[b.ID]
	}
	out := ToAPI(b, rev)
	out.Project = p.Name
	return out
}

func (s *server) apiListComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, ok := s.pick(w, r, "")
	if !ok {
		return
	}
	comments, err := p.F.FetchComments(r.Context(), id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	if comments == nil {
		comments = []Comment{}
	}
	writeJSON(w, http.StatusOK, comments)
}

// apiMermaid returns Mermaid text for ?root=<id>. With no root it returns
// an empty body: the UI builds the graph from /api/beads itself and uses
// this text only as a fallback. Errors soft-fail to a one-node flowchart,
// same as the ayo route, so the graph tab never breaks on a bd error.
func (s *server) apiMermaid(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	root := r.URL.Query().Get("root")
	if root == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !validID(root) {
		writeError(w, http.StatusBadRequest, "invalid root id")
		return
	}
	p, ok := s.pick(w, r, "")
	if !ok {
		return
	}
	text, err := p.F.FetchMermaid(r.Context(), root)
	if err != nil {
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		text = "flowchart TD\n  empty[\"No dependency tree available\"]\n  %% " + msg + "\n"
	}
	_, _ = io.WriteString(w, text)
}

type configResponse struct {
	WSPort        *int    `json:"ws_port"`
	WSPath        *string `json:"ws_path"`
	HasWebsockets bool    `json:"has_websockets"`
	ReadOnly      bool    `json:"read_only"`
	Project       string  `json:"project"`
	// Multi is true when the server was started with --projects.
	Multi bool `json:"multi"`
}

func (s *server) apiConfig(w http.ResponseWriter, r *http.Request) {
	project := ""
	if !s.multi {
		project = s.projects[0].Name
	}
	writeJSON(w, http.StatusOK, configResponse{ReadOnly: s.opts.ReadOnly, Project: project, Multi: s.multi})
}

type projectRow struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Registered  bool   `json:"registered"`
}

// apiProjects lists the configured projects. With no --projects file it is
// the one --dir.
func (s *server) apiProjects(w http.ResponseWriter, r *http.Request) {
	rows := make([]projectRow, 0, len(s.projects))
	for _, p := range s.projects {
		rows = append(rows, projectRow{
			Slug: p.Name, Name: p.Name, Description: p.F.Dir(), Path: p.F.Dir(),
			Enabled: true, Registered: true,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": rows})
}

// pick resolves which project a request is for. The name comes from the
// {project} path value, then the ?project= query, then bodyProject (the
// JSON body field). A single-project server ignores the name, as it
// always has. A multi-project server answers 400 when none is given
// (unless only one is configured) and 404 for an unknown name.
func (s *server) pick(w http.ResponseWriter, r *http.Request, bodyProject string) (*Project, bool) {
	if !s.multi {
		return &s.projects[0], true
	}
	name := r.PathValue("project")
	if name == "" {
		name = r.URL.Query().Get("project")
	}
	if name == "" {
		name = bodyProject
	}
	if name == "" {
		if len(s.projects) == 1 {
			return &s.projects[0], true
		}
		writeError(w, http.StatusBadRequest, "project is required (query ?project=, path, or body field)")
		return nil, false
	}
	for i := range s.projects {
		if s.projects[i].Name == name {
			return &s.projects[i], true
		}
	}
	writeError(w, http.StatusNotFound, "unknown project: "+name)
	return nil, false
}

// listProjects reads every project's beads in parallel and merges them in
// project order. A project that fails is returned in failed and left out.
func (s *server) listProjects(ctx context.Context, sel []Project) ([]APIBead, []error) {
	type res struct {
		beads []APIBead
		err   error
	}
	results := make([]res, len(sel))
	var wg sync.WaitGroup
	for i := range sel {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			beads, err := sel[i].F.FetchBeads(ctx)
			if err != nil {
				results[i].err = fmt.Errorf("%s: %w", sel[i].Name, err)
				return
			}
			list := ToAPIList(beads)
			for j := range list {
				list[j].Project = sel[i].Name
			}
			results[i].beads = list
		}(i)
	}
	wg.Wait()
	out := make([]APIBead, 0)
	var failed []error
	for _, r := range results {
		if r.err != nil {
			failed = append(failed, r.err)
			continue
		}
		out = append(out, r.beads...)
	}
	return out, failed
}

// write wraps a write endpoint with every gate, in this order:
//  1. --read-only: 405, before anything else runs.
//  2. Cross-site check: a JSON Content-Type is required (a plain HTML form
//     or a "simple" fetch from another site cannot send one without a CORS
//     preflight, which this server never answers), and an Origin header, if
//     sent, must match the Host. There is no auth, so this is what stops a
//     random web page from writing to a local bead database.
//  3. Options.AuthorizeWrite (the Casbin beadview/write action in iugum).
//
// Every attempt, allowed or refused, is logged with its action and path.
func (s *server) write(action string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.opts.ReadOnly {
			s.logf("write refused (read-only): %s %s %s", action, r.Method, r.URL.Path)
			w.Header().Set("Allow", "GET")
			writeError(w, http.StatusMethodNotAllowed, "beadview is read-only (--read-only): writes are disabled")
			return
		}
		if err := checkSameOrigin(r); err != nil {
			s.logf("write refused (%v): %s %s %s", err, action, r.Method, r.URL.Path)
			status := http.StatusForbidden
			if errors.Is(err, errContentType) {
				status = http.StatusUnsupportedMediaType
			}
			writeError(w, status, err.Error())
			return
		}
		if s.opts.AuthorizeWrite != nil {
			if err := s.opts.AuthorizeWrite(r.Context(), action); err != nil {
				s.logf("write refused (policy: %v): %s %s %s", err, action, r.Method, r.URL.Path)
				writeError(w, http.StatusForbidden, err.Error())
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		h(w, r)
	}
}

var errContentType = errors.New("writes need Content-Type: application/json")

func checkSameOrigin(r *http.Request) error {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]))
	if ct != "application/json" {
		return errContentType
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || u.Host != r.Host {
			return fmt.Errorf("cross-origin write refused (Origin %q, Host %q)", o, r.Host)
		}
	}
	return nil
}

type createRequest struct {
	Project      string     `json:"project"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Type         string     `json:"type"`
	Priority     flexString `json:"priority"`
	Dependencies []string   `json:"dependencies"`
}

type createResponse struct {
	ID               string   `json:"id"`
	Issue            APIBead  `json:"issue"`
	DependencyErrors []string `json:"dependency_errors,omitempty"`
}

// apiCreateBead runs `bd create`, then `bd dep add` once per dependency.
// A failed dependency does not undo the create (same as the ayo route);
// it is reported in dependency_errors and in the log.
func (s *server) apiCreateBead(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decodeBody(w, r, &req) {
		return
	}
	in := CreateInput{
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Type:        strings.TrimSpace(req.Type),
	}
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if req.Priority != "" {
		p, err := priorityToBD(string(req.Priority))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.Priority = p
	}
	p, ok := s.pick(w, r, req.Project)
	if !ok {
		return
	}
	var deps []string
	for _, d := range req.Dependencies {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if !validID(d) {
			writeError(w, http.StatusBadRequest, "invalid dependency id: "+d)
			return
		}
		deps = append(deps, d)
	}

	s.logf("write: create title=%q type=%q priority=%q deps=%v", in.Title, in.Type, in.Priority, deps)
	b, err := p.F.CreateBead(r.Context(), in)
	if err != nil {
		s.logf("write failed: create: %v", err)
		writeUpstreamError(w, err)
		return
	}
	s.logf("write ok: create %s", b.ID)
	resp := createResponse{ID: b.ID}
	for _, d := range deps {
		if err := p.F.AddDependency(r.Context(), b.ID, d); err != nil {
			s.logf("write failed: dep add %s -> %s: %v", b.ID, d, err)
			resp.DependencyErrors = append(resp.DependencyErrors, fmt.Sprintf("%s: %v", d, err))
			continue
		}
		s.logf("write ok: dep add %s -> %s", b.ID, d)
	}
	// Re-read so the response carries the new dependency edges.
	if len(deps) > 0 {
		if fresh, err := p.F.FetchBead(r.Context(), b.ID); err == nil {
			b = fresh
		}
	}
	resp.Issue = ToAPI(*b, nil)
	resp.Issue.Project = p.Name
	writeJSON(w, http.StatusCreated, resp)
}

type updateRequest struct {
	Project     string      `json:"project"`
	Status      *string     `json:"status"`
	Priority    *flexString `json:"priority"`
	Description *string     `json:"description"`
	Title       *string     `json:"title"`
	Type        *string     `json:"type"`
	Claim       bool        `json:"claim"`
}

// apiUpdateBead runs one `bd update` with only the fields the body sets.
func (s *server) apiUpdateBead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateRequest
	if !decodeBody(w, r, &req) {
		return
	}
	in := UpdateInput{
		Status:      trimPtr(req.Status),
		Description: req.Description,
		Title:       trimPtr(req.Title),
		Type:        trimPtr(req.Type),
		Claim:       req.Claim,
	}
	if in.Title != nil && *in.Title == "" {
		writeError(w, http.StatusBadRequest, "title cannot be empty")
		return
	}
	if in.Status != nil && *in.Status == "" {
		writeError(w, http.StatusBadRequest, "status cannot be empty")
		return
	}
	if req.Priority != nil {
		p, err := priorityToBD(string(*req.Priority))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.Priority = &p
	}
	if in.Empty() {
		writeError(w, http.StatusBadRequest, "no fields to update (status, priority, description, title, type, claim)")
		return
	}
	p, ok := s.pick(w, r, req.Project)
	if !ok {
		return
	}
	s.logf("write: update %s %s", id, redactDescription(in.Args()))
	b, err := p.F.UpdateBead(r.Context(), id, in)
	if err != nil {
		s.logf("write failed: update %s: %v", id, err)
		writeUpstreamError(w, err)
		return
	}
	s.logf("write ok: update %s status=%s", id, b.Status)
	writeJSON(w, http.StatusOK, s.reread(r.Context(), p, b))
}

type reasonRequest struct {
	Project string `json:"project"`
	Reason  string `json:"reason"`
}

func (s *server) apiCloseBead(w http.ResponseWriter, r *http.Request) {
	s.closeOrReopen(w, r, "close", func(p *Project) closeFn { return p.F.CloseBead })
}

func (s *server) apiReopenBead(w http.ResponseWriter, r *http.Request) {
	s.closeOrReopen(w, r, "reopen", func(p *Project) closeFn { return p.F.ReopenBead })
}

type closeFn = func(context.Context, string, string) (*Bead, error)

func (s *server) closeOrReopen(w http.ResponseWriter, r *http.Request, action string, pickFn func(*Project) closeFn) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req reasonRequest
	if !decodeOptionalBody(w, r, &req) {
		return
	}
	p, ok := s.pick(w, r, req.Project)
	if !ok {
		return
	}
	fn := pickFn(p)
	s.logf("write: %s %s reason=%q", action, id, req.Reason)
	b, err := fn(r.Context(), id, strings.TrimSpace(req.Reason))
	if err != nil {
		s.logf("write failed: %s %s: %v", action, id, err)
		writeUpstreamError(w, err)
		return
	}
	s.logf("write ok: %s %s status=%s", action, id, b.Status)
	writeJSON(w, http.StatusOK, s.reread(r.Context(), p, b))
}

type commentRequest struct {
	Project string `json:"project"`
	Text    string `json:"text"`
}

func (s *server) apiAddComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req commentRequest
	if !decodeBody(w, r, &req) {
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	p, ok := s.pick(w, r, req.Project)
	if !ok {
		return
	}
	s.logf("write: comment %s (%d bytes)", id, len(text))
	c, err := p.F.AddComment(r.Context(), id, text)
	if err != nil {
		s.logf("write failed: comment %s: %v", id, err)
		writeUpstreamError(w, err)
		return
	}
	s.logf("write ok: comment %s id=%s", id, c.ID)
	writeJSON(w, http.StatusCreated, c)
}

// redactDescription keeps descriptions out of the log (they can be long
// and may hold text the operator would not want in a server log).
func redactDescription(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "--description=") {
			a = fmt.Sprintf("--description=<%d bytes>", len(a)-len("--description="))
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, "invalid bead id")
		return "", false
	}
	return id, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// decodeOptionalBody accepts an empty body (close/reopen send only
// {project} or nothing).
func decodeOptionalBody(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeUpstreamError maps a bd failure to HTTP: "not found" style errors
// become 404, a refused close (open blockers) 409, everything else 502
// (the upstream is the bd child process).
func writeUpstreamError(w http.ResponseWriter, err error) {
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "not found") || strings.Contains(low, "no issue found") || strings.Contains(low, "no issues found") {
		writeError(w, http.StatusNotFound, msg)
		return
	}
	if strings.Contains(low, "cannot close") {
		writeError(w, http.StatusConflict, msg)
		return
	}
	writeError(w, http.StatusBadGateway, msg)
}
