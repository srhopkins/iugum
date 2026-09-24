package beadview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// apiFixture is newFakeServer plus a labelled bead with a "blocks" edge,
// so the list shape covers labels, assignee, closed_at and reverse blocks.
func apiFixture() *fakeFetcher {
	f := newFakeServer()
	blocker := Bead{
		ID: "task-2", Title: "Blocked task", Status: "closed", Priority: json.Number("4"),
		IssueType: "bug", Assignee: "sam", Labels: []string{"ui", "go"},
		CreatedAt: "2026-09-01T00:00:00Z", UpdatedAt: "2026-09-02T00:00:00Z", ClosedAt: "2026-09-03T00:00:00Z",
		Dependencies: []Dependency{{IssueID: "task-2", DependsOnID: "epic-1.1", Type: "blocks"}},
	}
	f.beads = append(f.beads, blocker)
	f.byID[blocker.ID] = &blocker
	return f
}

type apiClient struct {
	t   *testing.T
	srv *httptest.Server
}

func newAPIClient(t *testing.T, f Fetcher, opts Options) *apiClient {
	t.Helper()
	srv := httptest.NewServer(newTestHandler(f, opts))
	t.Cleanup(srv.Close)
	return &apiClient{t: t, srv: srv}
}

// do sends a request; body "" means no body. JSON writes carry the
// Content-Type the write gate requires.
func (c *apiClient) do(method, path, body string) (int, string, http.Header) {
	c.t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.srv.URL+path, rdr)
	if err != nil {
		c.t.Fatal(err)
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func decode[T any](t *testing.T, body string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	return v
}

func TestAPIListBeadsShape(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	code, body, hdr := c.do("GET", "/api/beads", "")
	if code != 200 {
		t.Fatalf("status = %d: %s", code, body)
	}
	if ct := hdr.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	// Decode as raw maps so the test checks the wire field names, not
	// just the Go struct.
	rows := decode[[]map[string]any](t, body)
	if len(rows) != 3 {
		t.Fatalf("got %d beads", len(rows))
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		byID[r["id"].(string)] = r
	}
	child := byID["epic-1.1"]
	if child["type"] != "task" || child["priority"] != "p0" || child["parent"] != "epic-1" {
		t.Errorf("child mapped wrong: %v", child)
	}
	if _, ok := child["issue_type"]; !ok {
		t.Errorf("issue_type should be kept alongside type")
	}
	if got := child["depends_on"].([]any); len(got) != 1 || got[0] != "epic-1" {
		t.Errorf("depends_on = %v, want flattened [epic-1]", got)
	}
	if got := child["dependencies"].([]any); len(got) != 1 || got[0] != "epic-1" {
		t.Errorf("dependencies = %v, want flattened [epic-1]", got)
	}
	if got := child["blocks"].([]any); len(got) != 1 || got[0] != "task-2" {
		t.Errorf("reverse blocks = %v, want [task-2]", got)
	}
	task := byID["task-2"]
	if task["priority"] != "p4" || task["assignee"] != "sam" || task["closed_at"] != "2026-09-03T00:00:00Z" {
		t.Errorf("task-2 mapped wrong: %v", task)
	}
	if got := task["labels"].([]any); len(got) != 2 {
		t.Errorf("labels = %v", got)
	}
	if got := task["blocked_by"].([]any); len(got) != 1 || got[0] != "epic-1.1" {
		t.Errorf("blocked_by = %v", got)
	}
	edges := task["dependency_edges"].([]any)
	if e := edges[0].(map[string]any); e["type"] != "blocks" {
		t.Errorf("dependency_edges lost the edge type: %v", edges)
	}
	// Empty lists are [] on the wire, never null (the UI calls .map).
	epic := byID["epic-1"]
	for _, k := range []string{"labels", "depends_on", "dependencies", "blocks", "blocked_by", "dependency_edges"} {
		if _, ok := epic[k].([]any); !ok {
			t.Errorf("epic %s = %#v, want []", k, epic[k])
		}
	}
}

func TestAPIListBeadsUpstreamError(t *testing.T) {
	f := apiFixture()
	f.failBeads = true
	c := newAPIClient(t, f, Options{})
	code, body, _ := c.do("GET", "/api/beads", "")
	if code != http.StatusBadGateway {
		t.Fatalf("status = %d", code)
	}
	if decode[map[string]string](t, body)["error"] == "" {
		t.Errorf("error body missing: %s", body)
	}
}

func TestAPIGetBead(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	code, body, _ := c.do("GET", "/api/bead/epic-1.1", "")
	if code != 200 {
		t.Fatalf("status = %d: %s", code, body)
	}
	b := decode[APIBead](t, body)
	if b.ID != "epic-1.1" || b.Type != "task" || b.Priority != "p0" {
		t.Errorf("bead = %+v", b)
	}
	if len(b.Blocks) != 1 || b.Blocks[0] != "task-2" {
		t.Errorf("single bead reverse blocks = %v", b.Blocks)
	}

	if code, _, _ := c.do("GET", "/api/bead/nope", ""); code != http.StatusNotFound {
		t.Errorf("missing bead status = %d, want 404", code)
	}
	if code, _, _ := c.do("GET", "/api/bead/-rf", ""); code != http.StatusBadRequest {
		t.Errorf("flag-like id status = %d, want 400", code)
	}
}

func TestAPIComments(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	code, body, _ := c.do("GET", "/api/bead/epic-1/comments", "")
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	got := decode[[]map[string]any](t, body)
	if len(got) != 1 || got[0]["author"] != "steve" || got[0]["text"] != "hello" {
		t.Errorf("comments = %v", got)
	}
	// No comments is [], not null.
	_, body, _ = c.do("GET", "/api/bead/epic-1.1/comments", "")
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("empty comments = %q, want []", body)
	}
}

func TestAPIMermaid(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	code, body, hdr := c.do("GET", "/api/mermaid", "")
	if code != 200 || body != "" || !strings.HasPrefix(hdr.Get("Content-Type"), "text/plain") {
		t.Errorf("no-root mermaid = %d %q %q", code, body, hdr.Get("Content-Type"))
	}
	_, body, _ = c.do("GET", "/api/mermaid?root=epic-1", "")
	if !strings.Contains(body, "epic-1") {
		t.Errorf("rooted mermaid = %q", body)
	}
	f.failMermaid = true
	code, body, _ = c.do("GET", "/api/mermaid?root=epic-1", "")
	if code != 200 || !strings.HasPrefix(body, "flowchart TD") {
		t.Errorf("mermaid error should soft-fail to a flowchart, got %d %q", code, body)
	}
}

func TestAPIConfigAndProjects(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	_, body, _ := c.do("GET", "/api/config", "")
	cfg := decode[map[string]any](t, body)
	if v, ok := cfg["ws_port"]; !ok || v != nil {
		t.Errorf("ws_port = %v, want null", v)
	}
	if cfg["read_only"] != false || cfg["project"] != "dir" {
		t.Errorf("config = %v", cfg)
	}

	_, body, _ = c.do("GET", "/api/projects", "")
	p := decode[struct {
		Projects []projectRow `json:"projects"`
	}](t, body)
	if len(p.Projects) != 1 || p.Projects[0].Slug != "dir" || p.Projects[0].Description != "/fake/dir" {
		t.Errorf("projects = %+v", p.Projects)
	}
}

func TestAPICreateBead(t *testing.T) {
	var logBuf bytes.Buffer
	f := apiFixture()
	c := newAPIClient(t, f, Options{Log: log.New(&logBuf, "", 0)})
	code, body, _ := c.do("POST", "/api/bead",
		`{"title":"  New one ","priority":"p1","type":"bug","description":"d","dependencies":["epic-1","","missing-9"],"project":"ignored"}`)
	if code != http.StatusCreated {
		t.Fatalf("status = %d: %s", code, body)
	}
	resp := decode[createResponse](t, body)
	if resp.ID != "new-1" || resp.Issue.Title != "New one" || resp.Issue.Priority != "p1" || resp.Issue.Type != "bug" {
		t.Errorf("create response = %+v", resp)
	}
	if len(f.creates) != 1 || f.creates[0].Priority != "1" || f.creates[0].Title != "New one" {
		t.Errorf("CreateBead input = %+v", f.creates)
	}
	if strings.Join(f.depAdds, ",") != "new-1->epic-1,new-1->missing-9" {
		t.Errorf("dep adds = %v", f.depAdds)
	}
	if len(resp.DependencyErrors) != 1 || !strings.Contains(resp.DependencyErrors[0], "missing-9") {
		t.Errorf("dependency_errors = %v", resp.DependencyErrors)
	}
	if len(resp.Issue.DependsOn) != 1 || resp.Issue.DependsOn[0] != "epic-1" {
		t.Errorf("created bead should be re-read with its deps: %v", resp.Issue.DependsOn)
	}
	logs := logBuf.String()
	for _, want := range []string{"write: create", "write ok: create new-1", "write failed: dep add new-1 -> missing-9"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q:\n%s", want, logs)
		}
	}
}

func TestAPICreateBeadValidation(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	cases := map[string]string{
		"no title":      `{"title":"  "}`,
		"bad priority":  `{"title":"x","priority":"p9"}`,
		"flag-like dep": `{"title":"x","dependencies":["--force"]}`,
		"not json":      `title=x`,
	}
	for name, body := range cases {
		if code, resp, _ := c.do("POST", "/api/bead", body); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d (%s), want 400", name, code, resp)
		}
	}
	if len(f.creates) != 0 {
		t.Errorf("invalid requests reached bd create: %v", f.creates)
	}
}

func TestAPICreateBeadNumericPriority(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	if code, body, _ := c.do("POST", "/api/bead", `{"title":"x","priority":3}`); code != http.StatusCreated {
		t.Fatalf("status = %d: %s", code, body)
	}
	if f.creates[0].Priority != "3" {
		t.Errorf("numeric priority = %q, want 3", f.creates[0].Priority)
	}
}

func TestAPICreateBeadUpstreamError(t *testing.T) {
	f := apiFixture()
	f.failWrite = true
	c := newAPIClient(t, f, Options{})
	if code, _, _ := c.do("POST", "/api/bead", `{"title":"x"}`); code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", code)
	}
}

func TestAPIUpdateBead(t *testing.T) {
	var logBuf bytes.Buffer
	f := apiFixture()
	c := newAPIClient(t, f, Options{Log: log.New(&logBuf, "", 0)})
	code, body, _ := c.do("PATCH", "/api/bead/epic-1", `{"status":"in_progress","priority":"p3","description":"secret text","project":"x"}`)
	if code != 200 {
		t.Fatalf("status = %d: %s", code, body)
	}
	b := decode[APIBead](t, body)
	if b.Status != "in_progress" || b.Priority != "p3" {
		t.Errorf("updated bead = %+v", b)
	}
	if len(f.updates) != 1 || f.updates[0] != "epic-1 --status=in_progress --priority=3 --description=secret text" {
		t.Errorf("update args = %v", f.updates)
	}
	if strings.Contains(logBuf.String(), "secret text") {
		t.Errorf("description text leaked into the log: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "write: update epic-1") {
		t.Errorf("update not logged: %s", logBuf.String())
	}

	// Claim alone is a valid update.
	if code, _, _ := c.do("PATCH", "/api/bead/epic-1", `{"claim":true}`); code != 200 {
		t.Errorf("claim status = %d", code)
	}
	if f.updates[1] != "epic-1 --claim" {
		t.Errorf("claim args = %q", f.updates[1])
	}
	// An empty description is an explicit clear, not "unset".
	c.do("PATCH", "/api/bead/epic-1", `{"description":""}`)
	if f.updates[2] != "epic-1 --description=" {
		t.Errorf("clear-description args = %q", f.updates[2])
	}
}

func TestAPIUpdateBeadValidation(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	for name, body := range map[string]string{
		"nothing":      `{"project":"x"}`,
		"bad priority": `{"priority":"high"}`,
		"empty title":  `{"title":" "}`,
		"empty status": `{"status":""}`,
	} {
		if code, _, _ := c.do("PATCH", "/api/bead/epic-1", body); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, code)
		}
	}
	if code, _, _ := c.do("PATCH", "/api/bead/nope", `{"status":"open"}`); code != http.StatusNotFound {
		t.Errorf("unknown bead status = %d, want 404", code)
	}
	if len(f.updates) != 1 { // only the unknown-bead call reached bd
		t.Errorf("updates = %v", f.updates)
	}
}

func TestAPICloseReopen(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	code, body, _ := c.do("POST", "/api/bead/epic-1/close", `{"reason":"done","project":"x"}`)
	if code != 200 || decode[APIBead](t, body).Status != "closed" {
		t.Fatalf("close = %d %s", code, body)
	}
	// Reopen with no body at all (the gate still needs the JSON header).
	code, body, _ = c.do("POST", "/api/bead/epic-1/reopen", "")
	if code != 200 || decode[APIBead](t, body).Status != "open" {
		t.Fatalf("reopen = %d %s", code, body)
	}
	if code, _, _ := c.do("POST", "/api/bead/epic-1.1/close", `{}`); code != http.StatusConflict {
		t.Errorf("blocked close = %d, want 409", code)
	}
	if strings.Join(f.closes, ",") != "epic-1:done,epic-1.1:" || strings.Join(f.reopens, ",") != "epic-1:" {
		t.Errorf("closes=%v reopens=%v", f.closes, f.reopens)
	}
}

func TestAPIAddComment(t *testing.T) {
	f := apiFixture()
	c := newAPIClient(t, f, Options{})
	code, body, _ := c.do("POST", "/api/bead/epic-1/comments", `{"text":"-looks good"}`)
	if code != http.StatusCreated {
		t.Fatalf("status = %d: %s", code, body)
	}
	if got := decode[Comment](t, body); got.Text != "-looks good" || got.IssueID != "epic-1" {
		t.Errorf("comment = %+v", got)
	}
	if code, _, _ := c.do("POST", "/api/bead/epic-1/comments", `{"text":"  "}`); code != http.StatusBadRequest {
		t.Errorf("empty comment status = %d, want 400", code)
	}
}

// writeCalls is every write endpoint with a body that would succeed.
var writeCalls = []struct{ method, path, body string }{
	{"POST", "/api/bead", `{"title":"x"}`},
	{"PATCH", "/api/bead/epic-1", `{"status":"closed"}`},
	{"POST", "/api/bead/epic-1/close", `{}`},
	{"POST", "/api/bead/epic-1/reopen", `{}`},
	{"POST", "/api/bead/epic-1/comments", `{"text":"hi"}`},
	{"POST", "/api/bd/bead", `{"title":"x"}`},
}

func TestAPIReadOnlyRefusesWritesWith405(t *testing.T) {
	var logBuf bytes.Buffer
	f := apiFixture()
	c := newAPIClient(t, f, Options{ReadOnly: true, Log: log.New(&logBuf, "", 0)})
	for _, w := range writeCalls {
		code, body, hdr := c.do(w.method, w.path, w.body)
		if code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", w.method, w.path, code)
		}
		if hdr.Get("Allow") != "GET" || !strings.Contains(decode[map[string]string](t, body)["error"], "read-only") {
			t.Errorf("%s %s: Allow=%q body=%s", w.method, w.path, hdr.Get("Allow"), body)
		}
	}
	if n := f.writes(); n != 0 {
		t.Errorf("read-only server made %d write calls", n)
	}
	if !strings.Contains(logBuf.String(), "write refused (read-only)") {
		t.Errorf("refusals not logged: %s", logBuf.String())
	}
	// Reads still work, and the UI can see the mode.
	if code, _, _ := c.do("GET", "/api/beads", ""); code != 200 {
		t.Errorf("read-only GET /api/beads = %d", code)
	}
	_, body, _ := c.do("GET", "/api/config", "")
	if decode[map[string]any](t, body)["read_only"] != true {
		t.Errorf("config read_only not true: %s", body)
	}
}

func TestAPIWriteNeedsJSONContentType(t *testing.T) {
	f := apiFixture()
	srv := httptest.NewServer(newTestHandler(f, Options{}))
	defer srv.Close()
	// A cross-site HTML form can only send these content types.
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", ""} {
		req, _ := http.NewRequest("POST", srv.URL+"/api/bead", strings.NewReader(`{"title":"x"}`))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: status = %d, want 415", ct, resp.StatusCode)
		}
	}
	if f.writes() != 0 {
		t.Errorf("non-JSON write reached bd: %v", f.creates)
	}
}

func TestAPIWriteRefusesCrossOrigin(t *testing.T) {
	f := apiFixture()
	srv := httptest.NewServer(newTestHandler(f, Options{}))
	defer srv.Close()
	send := func(origin string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/api/bead", strings.NewReader(`{"title":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := send("https://evil.example"); code != http.StatusForbidden {
		t.Errorf("cross-origin status = %d, want 403", code)
	}
	if code := send(srv.URL); code != http.StatusCreated {
		t.Errorf("same-origin status = %d, want 201", code)
	}
	if len(f.creates) != 1 {
		t.Errorf("creates = %v, want only the same-origin one", f.creates)
	}
}

func TestAPIWriteAuthorizeHook(t *testing.T) {
	f := apiFixture()
	var actions []string
	c := newAPIClient(t, f, Options{AuthorizeWrite: func(_ context.Context, action string) error {
		actions = append(actions, action)
		return errors.New("denied: beadview/write")
	}})
	for _, w := range writeCalls[:5] {
		if code, _, _ := c.do(w.method, w.path, w.body); code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", w.method, w.path, code)
		}
	}
	if strings.Join(actions, ",") != "create,update,close,reopen,comment" {
		t.Errorf("authorize actions = %v", actions)
	}
	if f.writes() != 0 {
		t.Errorf("denied writes reached bd")
	}
}

func TestAPIBdPrefixAlias(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	code, body, _ := c.do("GET", "/api/bd/beads", "")
	if code != 200 || len(decode[[]APIBead](t, body)) != 3 {
		t.Errorf("/api/bd/beads = %d %s", code, body)
	}
}

func TestAPIUnknownEndpointIsJSON404(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{UI: testUI()})
	code, body, hdr := c.do("GET", "/api/nope", "")
	if code != 404 || hdr.Get("Content-Type") != "application/json" {
		t.Errorf("unknown API = %d %q %s", code, hdr.Get("Content-Type"), body)
	}
	if strings.Contains(body, "<html") {
		t.Errorf("unknown API path fell through to the SPA")
	}
}

func testUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":       {Data: []byte("<html>SPA INDEX</html>")},
		"assets/app-1.js":  {Data: []byte("console.log(1)")},
		"favicon.svg":      {Data: []byte("<svg/>")},
		"assets/app-1.css": {Data: []byte("body{}")},
	}
}

func TestSPAServesIndexAndAssets(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{UI: testUI()})
	for _, p := range []string{"/", "/index.html", "/some/client/route", "/activity?view=graph"} {
		code, body, hdr := c.do("GET", p, "")
		if code != 200 || body != "<html>SPA INDEX</html>" {
			t.Errorf("GET %s = %d %q, want index.html", p, code, body)
		}
		if !strings.HasPrefix(hdr.Get("Content-Type"), "text/html") {
			t.Errorf("GET %s Content-Type = %q", p, hdr.Get("Content-Type"))
		}
	}
	code, body, hdr := c.do("GET", "/assets/app-1.js", "")
	if code != 200 || body != "console.log(1)" || !strings.Contains(hdr.Get("Cache-Control"), "immutable") {
		t.Errorf("asset = %d %q %q", code, body, hdr.Get("Cache-Control"))
	}
	if code, body, _ := c.do("GET", "/favicon.svg", ""); code != 200 || body != "<svg/>" {
		t.Errorf("root file = %d %q", code, body)
	}
	// A missing hashed asset is a real 404, not index.html.
	if code, _, _ := c.do("GET", "/assets/gone-2.js", ""); code != 404 {
		t.Errorf("missing asset = %d, want 404", code)
	}
}

func TestSPAPlaceholderWithoutUI(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{})
	code, body, _ := c.do("GET", "/", "")
	if code != 200 || !strings.Contains(body, "/legacy/") {
		t.Errorf("placeholder = %d %s", code, body)
	}
}

func TestOldRoutesRedirect(t *testing.T) {
	c := newAPIClient(t, apiFixture(), Options{UI: testUI()})
	cases := map[string]string{
		"/bead/epic-1.1": "/?bead=epic-1.1",
		"/graph":         "/legacy/graph",
		"/tree?q=x":      "/legacy/tree?q=x",
		"/legacy":        "/legacy/",
	}
	for from, want := range cases {
		code, _, hdr := c.do("GET", from, "")
		if code != http.StatusFound && code != http.StatusMovedPermanently {
			t.Errorf("GET %s = %d, want a redirect", from, code)
		}
		if got := hdr.Get("Location"); got != want {
			t.Errorf("GET %s Location = %q, want %q", from, got, want)
		}
	}
	// The legacy pages themselves still render.
	for _, p := range []string{"/legacy/", "/legacy/tree", "/legacy/bead/epic-1", "/legacy/graph"} {
		if code, _, _ := c.do("GET", p, ""); code != 200 {
			t.Errorf("GET %s = %d", p, code)
		}
	}
}

func TestPriorityToBD(t *testing.T) {
	for in, want := range map[string]string{"p0": "0", "P4": "4", "2": "2", " p1 ": "1"} {
		if got, err := priorityToBD(in); err != nil || got != want {
			t.Errorf("priorityToBD(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "p5", "high", "p12", "-1"} {
		if _, err := priorityToBD(bad); err == nil {
			t.Errorf("priorityToBD(%q) should fail", bad)
		}
	}
}

func TestUpdateInputArgsUseEqualsForm(t *testing.T) {
	v := "--force"
	got := UpdateInput{Title: &v}.Args()
	if len(got) != 1 || got[0] != "--title=--force" {
		t.Errorf("Args() = %v: a flag-like value must stay inside --title=", got)
	}
}
