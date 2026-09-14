package agentdesk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagedHomeConnection(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/messages" || r.Header.Get("Origin") != "" {
			t.Error("incorrect forwarded request")
		}
		w.Write([]byte(`[]`))
	}))
	defer up.Close()
	s, e := New(Config{DataDir: t.TempDir(), HideDefaultAgent: true, Connections: map[string]func(context.Context) (string, error){"helper": func(context.Context) (string, error) { return up.URL, nil }}})
	if e != nil {
		t.Fatal(e)
	}
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://127.0.0.1:3850"+path, nil)
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request("/api/agents"); w.Code != 200 || !strings.Contains(w.Body.String(), "helper") {
		t.Fatal(w.Body.String())
	}
	if w := request("/api/agents/helper/messages"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request("/api/agents/helper/runtime/stop"); w.Code != 404 {
		t.Fatal("forwarded runtime control")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
