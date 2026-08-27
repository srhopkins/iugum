package sqladapt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDashboardSeedAndPut(t *testing.T) {
	s := openTest(t)
	dir := t.TempDir()
	h := NewHandler(s, HandlerOptions{DataDir: dir})

	got := httptest.NewRecorder()
	h.ServeHTTP(got, httptest.NewRequest(http.MethodGet, "/homelab-dashboard.json", nil))
	if got.Code != http.StatusOK {
		t.Fatalf("get %d %s", got.Code, got.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["kind"] != "Dashboard" {
		t.Fatalf("kind %v", raw["kind"])
	}
	seed := filepath.Join(dir, "dashboards", "homelab-dashboard.json")
	if _, err := os.Stat(seed); err != nil {
		t.Fatalf("seed missing: %v", err)
	}

	raw["spec"].(map[string]any)["display"] = map[string]any{"name": "edited"}
	body, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	put := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/homelab-dashboard.json", bytes.NewReader(body))
	h.ServeHTTP(put, req)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}

	again := httptest.NewRecorder()
	h.ServeHTTP(again, httptest.NewRequest(http.MethodGet, "/homelab-dashboard.json", nil))
	if !bytes.Contains(again.Body.Bytes(), []byte(`"name": "edited"`)) && !bytes.Contains(again.Body.Bytes(), []byte(`"name":"edited"`)) {
		t.Fatalf("saved %s", again.Body.String())
	}

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodPut, "/homelab-dashboard.json", bytes.NewReader([]byte(`{"kind":"nope"}`))))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad put %d", bad.Code)
	}
}

func TestDashboardPutWithoutDataDir(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	got := httptest.NewRecorder()
	h.ServeHTTP(got, httptest.NewRequest(http.MethodPut, "/homelab-dashboard.json", bytes.NewReader([]byte(`{"kind":"Dashboard","spec":{}}`))))
	if got.Code != http.StatusNotImplemented {
		t.Fatalf("status %d %s", got.Code, got.Body.String())
	}
}

func TestDashboardList(t *testing.T) {
	s := openTest(t)
	dir := t.TempDir()
	h := NewHandler(s, HandlerOptions{DataDir: dir})
	got := httptest.NewRecorder()
	h.ServeHTTP(got, httptest.NewRequest(http.MethodGet, "/dashboards", nil))
	if got.Code != http.StatusOK {
		t.Fatalf("list %d %s", got.Code, got.Body.String())
	}
	if !bytes.Contains(got.Body.Bytes(), []byte("homelab-dashboard.json")) {
		t.Fatalf("list %s", got.Body.String())
	}
	one := httptest.NewRecorder()
	h.ServeHTTP(one, httptest.NewRequest(http.MethodGet, "/dashboards/homelab-dashboard.json", nil))
	if one.Code != http.StatusOK {
		t.Fatalf("one %d %s", one.Code, one.Body.String())
	}
}
