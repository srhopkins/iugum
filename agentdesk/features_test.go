package agentdesk

import (
	"net/http/httptest"
	"testing"
)

func TestDisabledFeatureRoutes(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), DisableChat: true, DisableSearch: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/search", "/api/agents", "/api/messages", "/.proxy/iugum/api/search", "/api/approvals"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:3850"+path, nil))
		if w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
