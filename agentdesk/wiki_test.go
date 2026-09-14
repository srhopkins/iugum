package agentdesk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeWikiIsTopLevelAndAPISeparate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<main id=sb-main>Native wiki "+r.URL.Path+"</main>")
	}))
	defer upstream.Close()
	s, e := New(Config{Name: "test", DataDir: t.TempDir(), WikiURL: upstream.URL, WikiIntegrated: true, Check: func(context.Context, string, string) error { return nil }})
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ path, want string }{{"/.proxy/iugum/assets/ui-styles.js", "export const panelCSS"}, {"/.proxy/iugum/assets/ui-controls.js", "export function actionMenu"}, {"/", "Native wiki /"}, {"/Commitments", "Native wiki /Commitments"}, {"/.fs/Commitments.md", "Native wiki /.fs/Commitments.md"}, {"/.proxy/iugum/api/status", `"name":"test"`}, {"/api/status", `"name":"test"`}} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:3850"+tc.path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if tc.path == "/" && strings.Contains(w.Body.String(), "<iframe") {
			t.Fatal("wiki wrapped in iframe")
		}
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:3850/.proxy/iugum/api/chat", strings.NewReader(`{"text":"status"}`))
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
}
