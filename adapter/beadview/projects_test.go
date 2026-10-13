package beadview

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func multiClient(t *testing.T, opts Options) (*apiClient, *fakeFetcher, *fakeFetcher) {
	t.Helper()
	a, b := newFakeServer(), newFakeServer()
	// Same IDs in both fakes on purpose: only the project tells them apart.
	b.beads[0].Title = "Other project bead"
	b.byID[b.beads[0].ID].Title = "Other project bead"
	h := NewMultiHandler([]Project{{Name: "alpha", F: a}, {Name: "beta", F: b}}, opts)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &apiClient{t: t, srv: srv}, a, b
}

func TestMultiListAllAndFiltered(t *testing.T) {
	c, _, _ := multiClient(t, Options{})
	code, body, _ := c.do("GET", "/api/beads", "")
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	all := decode[[]APIBead](t, body)
	counts := map[string]int{}
	for _, b := range all {
		if b.Project == "" {
			t.Fatalf("bead %s has no project", b.ID)
		}
		counts[b.Project]++
	}
	if counts["alpha"] == 0 || counts["alpha"] != counts["beta"] {
		t.Fatalf("counts = %v", counts)
	}
	_, body, _ = c.do("GET", "/api/beads?project=beta", "")
	for _, b := range decode[[]APIBead](t, body) {
		if b.Project != "beta" {
			t.Fatalf("filter leaked %s", b.Project)
		}
	}
	if code, _, _ = c.do("GET", "/api/beads?project=nope", ""); code != 404 {
		t.Fatalf("unknown project status %d", code)
	}
}

func TestMultiBeadByProjectPath(t *testing.T) {
	c, a, _ := multiClient(t, Options{})
	id := a.beads[0].ID
	_, body, _ := c.do("GET", "/api/bead/beta/"+id, "")
	got := decode[APIBead](t, body)
	if got.Project != "beta" || got.Title != "Other project bead" {
		t.Fatalf("got %+v", got)
	}
	if code, _, _ := c.do("GET", "/api/bead/"+id, ""); code != 400 {
		t.Fatalf("no project should be 400, got %d", code)
	}
}

func TestMultiWriteRoutesByProject(t *testing.T) {
	c, a, b := multiClient(t, Options{})
	id := a.beads[0].ID
	code, body, _ := c.do("PATCH", "/api/bead/beta/"+id, `{"status":"in_progress"}`)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if a.writes() != 0 || b.writes() != 1 {
		t.Fatalf("writes alpha=%d beta=%d", a.writes(), b.writes())
	}
	// The body field routes too (the old UI path).
	code, _, _ = c.do("PATCH", "/api/bead/"+id, `{"project":"alpha","status":"open"}`)
	if code != 200 || a.writes() != 1 {
		t.Fatalf("body routing: status %d, alpha writes %d", code, a.writes())
	}
}

func TestMultiReadOnly(t *testing.T) {
	c, _, _ := multiClient(t, Options{ReadOnly: true})
	if code, _, _ := c.do("PATCH", "/api/bead/alpha/x-1", `{"status":"open"}`); code != 405 {
		t.Fatalf("status %d", code)
	}
}

func TestMultiProjectsList(t *testing.T) {
	c, _, _ := multiClient(t, Options{})
	_, body, _ := c.do("GET", "/api/projects", "")
	if !strings.Contains(body, `"name":"alpha"`) || !strings.Contains(body, `"name":"beta"`) {
		t.Fatal(body)
	}
	_, body, _ = c.do("GET", "/api/config", "")
	if !strings.Contains(body, `"multi":true`) {
		t.Fatal(body)
	}
}

func TestParseProjects(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ParseProjects([]byte("- name: a\n  path: a\n"), dir)
	if err != nil || len(got) != 1 || got[0].Path != filepath.Join(dir, "a") {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := ParseProjects([]byte("projects:\n- {name: a, path: a}\n"), dir); err != nil {
		t.Fatalf("wrapped form: %v", err)
	}
	for _, bad := range []string{
		"- {name: a, path: a}\n- {name: a, path: a}\n",
		"- {name: 'bad name', path: a}\n",
		"- {name: b, path: missing}\n",
		"[]\n",
	} {
		if _, err := ParseProjects([]byte(bad), dir); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
