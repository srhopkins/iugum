package agentdesk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHold = "0123456789abcdef0123456789abcdef"

func draftServer(t *testing.T) (*Server, *int) {
	t.Helper()
	promoted := 0
	s, e := New(Config{Name: "fixture", DataDir: t.TempDir(), AllowPTTDrafts: true, Chat: func(ctx context.Context, text string) (string, error) {
		if Preparing(ctx) {
			if !strings.Contains(ConversationSnapshot(ctx), text) {
				t.Error("normal context missing proposed user text")
			}
			return "", SetPreparedValue(ctx, "private answer")
		}
		if PreparedValue(ctx) != nil {
			promoted++
		}
		return "accepted " + text, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	return s, &promoted
}
func prepareID(t *testing.T, s *Server, text string) string {
	t.Helper()
	w := req(s, "POST", "/api/draft", `{"operation":"prepare","hold_id":"`+testHold+`","revision":1,"text":"`+text+`"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"draft_id"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	return out.ID
}
func TestDraftDoesNotPersistAndMatchingReleasePromotes(t *testing.T) {
	s, promoted := draftServer(t)
	before, e := os.ReadFile(filepath.Join(s.cfg.DataDir, "workspace.json"))
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	id := prepareID(t, s, "question")
	after, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "workspace.json"))
	if string(before) != string(after) || len(s.data.Messages) != 0 {
		t.Fatal("preparation touched conversation")
	}
	w := req(s, "POST", "/api/chat", `{"text":"question","hold_id":"`+testHold+`","draft_id":"`+id+`"}`)
	if w.Code != 200 || *promoted != 1 || len(s.data.Messages) != 2 {
		t.Fatal(w.Code, *promoted, len(s.data.Messages))
	}
	w = req(s, "POST", "/api/chat", `{"text":"question","hold_id":"`+testHold+`","draft_id":"`+id+`"}`)
	if w.Code != 409 || *promoted != 1 || len(s.data.Messages) != 2 {
		t.Fatal("release replay accepted")
	}
}
func TestDraftChangedQuestionAndContextFallBack(t *testing.T) {
	for _, change := range []string{"question", "context"} {
		t.Run(change, func(t *testing.T) {
			s, promoted := draftServer(t)
			id := prepareID(t, s, "question")
			text := "question"
			if change == "question" {
				text = "new question"
			} else {
				req(s, "POST", "/api/chat", `{"text":"another turn"}`)
			}
			w := req(s, "POST", "/api/chat", `{"text":"`+text+`","hold_id":"`+testHold+`","draft_id":"`+id+`"}`)
			if w.Code != 200 || *promoted != 0 {
				t.Fatal("stale draft promoted", w.Code, *promoted)
			}
		})
	}
}
func TestDraftCancellationLeavesNoSession(t *testing.T) {
	s, _ := draftServer(t)
	_ = prepareID(t, s, "question")
	w := req(s, "POST", "/api/draft", `{"operation":"cancel","hold_id":"`+testHold+`"}`)
	if w.Code != 200 || s.draft != nil || len(s.data.Messages) != 0 {
		t.Fatal("draft not discarded")
	}
	w = req(s, "POST", "/api/draft", `{"operation":"prepare","hold_id":"`+testHold+`","revision":2,"text":"late"}`)
	if w.Code != 409 {
		t.Fatal("late revision resurrected hold", w.Code)
	}
}
func TestDraftOffByDefault(t *testing.T) {
	s, _ := draftServer(t)
	s.cfg.AllowPTTDrafts = false
	if w := req(s, "POST", "/api/draft", `{"operation":"prepare","hold_id":"`+testHold+`","revision":1,"text":"question"}`); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
