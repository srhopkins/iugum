package beadview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeFetcher is a Fetcher that never touches a subprocess, so handler tests
// need no built iugum binary and no real .beads directory.
type fakeFetcher struct {
	beads     []Bead
	byID      map[string]*Bead
	comments  map[string][]Comment
	graphHTML string
	failBeads bool

	failMermaid bool
	failWrite   bool

	creates     []CreateInput
	depAdds     []string
	updates     []string
	closes      []string
	reopens     []string
	commentAdds []string
}

func (f *fakeFetcher) FetchBeads(context.Context) ([]Bead, error) {
	if f.failBeads {
		return nil, errors.New("boom")
	}
	return f.beads, nil
}

func (f *fakeFetcher) FetchBead(_ context.Context, id string) (*Bead, error) {
	b, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found: " + id)
	}
	return b, nil
}

func (f *fakeFetcher) FetchComments(_ context.Context, id string) ([]Comment, error) {
	return f.comments[id], nil
}

func (f *fakeFetcher) FetchGraphHTML(context.Context) (string, error) {
	return f.graphHTML, nil
}

func (f *fakeFetcher) FetchStatus(context.Context) (string, error) { return "", nil }

func (f *fakeFetcher) Dir() string { return "/fake/dir" }

func (f *fakeFetcher) FetchMermaid(_ context.Context, root string) (string, error) {
	if f.failMermaid {
		return "", errors.New("mermaid boom")
	}
	return "flowchart TD\n  " + root + "\n", nil
}

func (f *fakeFetcher) CreateBead(_ context.Context, in CreateInput) (*Bead, error) {
	f.creates = append(f.creates, in)
	if f.failWrite {
		return nil, errors.New("write boom")
	}
	b := &Bead{ID: "new-1", Title: in.Title, Description: in.Description, IssueType: in.Type,
		Priority: json.Number(in.Priority), Status: "open"}
	f.byID[b.ID] = b
	return b, nil
}

func (f *fakeFetcher) AddDependency(_ context.Context, id, dependsOn string) error {
	f.depAdds = append(f.depAdds, id+"->"+dependsOn)
	if _, ok := f.byID[dependsOn]; !ok {
		return errors.New("issue " + dependsOn + " not found")
	}
	b := f.byID[id]
	b.Dependencies = append(b.Dependencies, Dependency{IssueID: id, DependsOnID: dependsOn, Type: "blocks"})
	return nil
}

func (f *fakeFetcher) UpdateBead(_ context.Context, id string, in UpdateInput) (*Bead, error) {
	f.updates = append(f.updates, id+" "+strings.Join(in.Args(), " "))
	b, ok := f.byID[id]
	if !ok {
		return nil, errors.New("no issue found matching " + id)
	}
	cp := *b
	if in.Status != nil {
		cp.Status = *in.Status
	}
	if in.Priority != nil {
		cp.Priority = json.Number(*in.Priority)
	}
	return &cp, nil
}

func (f *fakeFetcher) CloseBead(_ context.Context, id, reason string) (*Bead, error) {
	f.closes = append(f.closes, id+":"+reason)
	if id == "epic-1.1" {
		return nil, errors.New("cannot close epic-1.1: blocked by open issues [x] (use --force to override)")
	}
	b, ok := f.byID[id]
	if !ok {
		return nil, errors.New("no issue found matching " + id)
	}
	cp := *b
	cp.Status, cp.CloseReason = "closed", reason
	return &cp, nil
}

func (f *fakeFetcher) ReopenBead(_ context.Context, id, reason string) (*Bead, error) {
	f.reopens = append(f.reopens, id+":"+reason)
	b, ok := f.byID[id]
	if !ok {
		return nil, errors.New("no issue found matching " + id)
	}
	cp := *b
	cp.Status = "open"
	return &cp, nil
}

func (f *fakeFetcher) AddComment(_ context.Context, id, text string) (*Comment, error) {
	f.commentAdds = append(f.commentAdds, id+":"+text)
	return &Comment{ID: "c-new", IssueID: id, Author: "tester", Text: text}, nil
}

// writes counts every write call the handler made, for read-only tests.
func (f *fakeFetcher) writes() int {
	return len(f.creates) + len(f.depAdds) + len(f.updates) + len(f.closes) + len(f.reopens) + len(f.commentAdds)
}

func newTestHandler(f Fetcher, opts Options) http.Handler {
	if opts.Log == nil {
		opts.Log = log.New(io.Discard, "", 0)
	}
	return NewHandler(f, opts)
}

func newFakeServer() *fakeFetcher {
	epic := Bead{ID: "epic-1", Title: "Epic <one>", Status: "open", Priority: json.Number("1"), IssueType: "epic"}
	child := Bead{
		ID: "epic-1.1", Title: "Child of epic", Status: "in_progress", Priority: json.Number("0"),
		IssueType: "task", Parent: "epic-1",
		Dependencies: []Dependency{{IssueID: "epic-1.1", DependsOnID: "epic-1", Type: "parent-child"}},
	}
	return &fakeFetcher{
		beads: []Bead{epic, child},
		byID:  map[string]*Bead{"epic-1": &epic, "epic-1.1": &child},
		comments: map[string][]Comment{
			"epic-1": {{ID: "c1", Author: "steve", Text: "hello"}},
		},
		graphHTML: "<html><body>graph</body></html>",
	}
}

func TestHandlerList(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(newFakeServer(), Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "epic-1.1") {
		t.Errorf("list page missing bead id: %s", body)
	}
	// Title text must be HTML-escaped, not injected raw.
	if !strings.Contains(body, "Epic &lt;one&gt;") {
		t.Errorf("list page did not escape title: %s", body)
	}
}

func TestHandlerListFilterByStatus(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(newFakeServer(), Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/?status=in_progress")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp)
	if strings.Contains(body, "epic-1</a>") {
		t.Errorf("status filter did not exclude epic-1: %s", body)
	}
	if !strings.Contains(body, "epic-1.1") {
		t.Errorf("status filter dropped the matching bead: %s", body)
	}
}

func TestHandlerListUpstreamError(t *testing.T) {
	f := newFakeServer()
	f.failBeads = true
	srv := httptest.NewServer(newTestHandler(f, Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestHandlerDetail(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(newFakeServer(), Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/bead/epic-1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "hello") {
		t.Errorf("detail page missing comment text: %s", body)
	}
}

func TestHandlerDetailNotFound(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(newFakeServer(), Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/bead/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestHandlerGraph(t *testing.T) {
	srv := httptest.NewServer(newTestHandler(newFakeServer(), Options{}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/legacy/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := readAll(t, resp)
	if body != "<html><body>graph</body></html>" {
		t.Errorf("graph page not served verbatim: %s", body)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}
