package agentplatform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestCatalogConfiguredProviders(t *testing.T) {
	home := t.TempDir()
	configured := t.TempDir()
	id := "11111111-1111-4111-8111-111111111111"
	for _, root := range []string{filepath.Join(home, ".claude"), configured} {
		dir := filepath.Join(root, "projects", "project")
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(`{"type":"user","cwd":"/project","message":{"content":"hello"}}`+"\n"), 0600)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "opencode" || p != "private" {
			t.Error("missing auth")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/session" {
			t.Error(r.URL.Path)
		}
		json.NewEncoder(w).Encode([]any{map[string]any{"id": "ses_test", "directory": "/project", "title": "configured server", "time": map[string]int64{"updated": 1}}})
	}))
	defer api.Close()
	s := &Server{cfg: Config{Home: home}, providers: map[string]contract.AgentSessionProvider{"Claude": ClaudeProvider{ConfigDir: configured}, "OpenCode": OpenCodeHTTPProvider{BaseURL: api.URL, Password: "private"}}}
	got := s.catalog(context.Background()).(map[string]any)
	rows := got["sessions"].([]CatalogEntry)
	if len(rows) != 2 {
		t.Fatalf("catalog=%v", got)
	}
	delete(s.providers, "Claude")
	delete(s.providers, "OpenCode")
	got = s.catalog(context.Background()).(map[string]any)
	if len(got["sessions"].([]CatalogEntry)) != 0 {
		t.Fatalf("ambient sessions discovered: %v", got)
	}
}
