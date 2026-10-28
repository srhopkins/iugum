package beadsadapt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestActivityEndpointRequiresConfiguredProjectAndIsReadOnly(t *testing.T) {
	calls := 0
	run := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		calls++
		if dir != "/canonical/a" {
			t.Fatal(dir)
		}
		return []byte(`[{"id":"same","assignee":"worker","status":"in_progress","metadata":{"private":"hidden","iugum.activity":{"actor":"worker","state":"working","seen_at":"2026-10-20T00:00:00Z","expires_at":"2026-10-20T00:02:00Z"}}}]`), nil
	}
	now := func() time.Time { return time.Date(2026, 10, 20, 0, 1, 0, 0, time.UTC) }
	h := activityHandler(http.NotFoundHandler(), []activityProject{{"a", "/canonical/a"}, {"b", "/canonical/b"}}, run, now)
	for _, path := range []string{"/api/iugum/activity", "/api/iugum/activity?project=other"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("unscoped query executed")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/iugum/activity?project=a", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/iugum/activity?project=a", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"working"`) || strings.Contains(w.Body.String(), "private") {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestActivityPollReprojectsExpiryAndProjectScope(t *testing.T) {
	at := time.Date(2026, 10, 20, 0, 1, 0, 0, time.UTC)
	run := func(_ context.Context, dir string, _ ...string) ([]byte, error) {
		actor := "alpha-worker"
		if dir == "/beta" {
			actor = "beta-worker"
		}
		return []byte(`[{"id":"same","assignee":"` + actor + `","status":"in_progress","metadata":{"iugum.activity":{"actor":"` + actor + `","state":"working","seen_at":"2026-10-20T00:00:00Z","expires_at":"2026-10-20T00:02:00Z"}}}]`), nil
	}
	h := activityHandler(http.NotFoundHandler(), []activityProject{{"alpha", "/alpha"}, {"beta", "/beta"}}, run, func() time.Time { return at })
	get := func(project string) string {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/iugum/activity?project="+project, nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Body.String()
	}
	alpha := get("alpha")
	beta := get("beta")
	if !strings.Contains(alpha, "alpha-worker") || strings.Contains(alpha, "beta-worker") || !strings.Contains(beta, "beta-worker") {
		t.Fatal("cross-project activity leak")
	}
	at = at.Add(2 * time.Minute)
	if body := get("alpha"); !strings.Contains(body, `"state":"stale"`) {
		t.Fatal("poll retained expired working", body)
	}
}
