package agentdesk

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationSearchSavedMessagesAndContext(t *testing.T) {
	dir := t.TempDir()
	id := "11111111111111111111111111111111"
	other := "22222222222222222222222222222222"
	os.MkdirAll(filepath.Join(dir, "conversations", id), 0700)
	os.MkdirAll(filepath.Join(dir, "conversations", other), 0700)
	os.WriteFile(filepath.Join(dir, "workspace.json"), []byte(`{"conversation_title":"Main title","messages":[{"role":"user","text":"needle workspace"}]}`), 0600)
	os.WriteFile(filepath.Join(dir, "conversations", id, "workspace.json"), []byte(`{"conversation_title":"Other title","messages":[{"role":"assistant","text":"different content"}]}`), 0600)
	os.WriteFile(filepath.Join(dir, "conversations", id, "context.json"), []byte(`[{"role":"assistant","content":"needle context"},{"role":"tool","content":"secret tool payload"}]`), 0600)
	os.WriteFile(filepath.Join(dir, "conversations", other, "context.json"), []byte(`[{"role":"user","content":"context only"}]`), 0600)
	s, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) string {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:3850/api/"+path, nil))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	hits := get("conversations?q=needle")
	if !strings.Contains(hits, "Main title") || !strings.Contains(hits, "Other title") || !strings.Contains(hits, `"readOnly":"context"`) {
		t.Fatal(hits)
	}
	if strings.Contains(get("conversations?q=secret"), "Other title") {
		t.Fatal("tool output searchable")
	}
	snapshot := get("conversations?transcript=" + id)
	if !strings.Contains(snapshot, "needle context") || strings.Contains(snapshot, "secret tool") {
		t.Fatal(snapshot)
	}
	if !strings.Contains(get("conversations?q=context%20only"), other) {
		t.Fatal("context-only session absent")
	}
	if strings.Contains(get("conversations?q=%22%20OR%20%22"), "Main title") {
		t.Fatal("query not literal")
	}
}
