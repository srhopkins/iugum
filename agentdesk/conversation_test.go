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
