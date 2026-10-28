package agentdesk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConversationTitlePersistenceAndPolicy(t *testing.T) {
	cfg := Config{DataDir: t.TempDir()}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := func(s *Server, method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:3850/api/conversation", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request(s, "POST", `{"title":"A saved title"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", ""); !strings.Contains(w.Body.String(), "A saved title") {
		t.Fatal(w.Body.String())
	}
	cfg.Check = func(_ context.Context, resource, action string) error {
		if action == "write" {
			return errors.New("denied")
		}
		return nil
	}
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w := request(s, "POST", `{"title":"changed"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", ""); !strings.Contains(w.Body.String(), "A saved title") {
		t.Fatal(w.Body.String())
	}
}

func TestSeparateConversationHistory(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), SeparateConversations: true, PassthroughChat: true, Chat: func(ctx context.Context, text string) (string, error) { return ConversationID(ctx) + ":" + text, nil }}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:3850/api/"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	w := request("POST", "conversations", `{}`)
	var created map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"]
	request("POST", "chat?conversation="+id, `{"text":"private to this chat"}`)
	if strings.Contains(request("GET", "messages", "").Body.String(), "private to this chat") {
		t.Fatal("history leaked into main")
	}
	if !strings.Contains(request("GET", "messages?conversation="+id, "").Body.String(), id+":private") {
		t.Fatal("runtime missing conversation ID")
	}
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request("GET", "messages?conversation="+id, "").Body.String(), "private to this chat") {
		t.Fatal("conversation did not survive restart")
	}
}

func TestConversationApprovalDecisionStaysInChild(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), SeparateConversations: true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:3850/api/conversations", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var created map[string]string
	json.Unmarshal(w.Body.Bytes(), &created)
	child, err := s.conversationServer(created["id"])
	if err != nil {
		t.Fatal(err)
	}
	a, err := child.RequestApproval(context.Background(), ApprovalRequest{Title: "child action", Action: "test"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"decision": "reject", "digest": a.Digest})
	r = httptest.NewRequest("POST", "http://127.0.0.1:3850/api/approvals/"+a.ID+"?conversation="+created["id"], strings.NewReader(string(body)))
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("child decision went to main: %d %s", w.Code, w.Body.String())
	}
	child.mu.Lock()
	items, err := child.approvalsLocked()
	child.mu.Unlock()
	if err != nil || len(items) != 1 || items[0].Status != "rejected" {
		t.Fatal("child not resolved")
	}
	s.mu.Lock()
	main, _ := s.approvalsLocked()
	s.mu.Unlock()
	if len(main) != 0 {
		t.Fatal("main modified")
	}
}
