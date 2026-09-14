package agentdesk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func req(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:3850"+path, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestPersistCommitmentAndChat(t *testing.T) {
	c := Config{DataDir: t.TempDir()}
	s, e := New(c)
	if e != nil {
		t.Fatal(e)
	}
	w := req(s, "POST", "/api/chat", `{"text":"/commit Ship clear tickets"}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s, e = New(c)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.data.Messages) != 2 || len(s.data.Commitments) != 1 {
		t.Fatal(s.data)
	}
	w = req(s, "POST", "/api/chat", `{"text":"status"}`)
	if !strings.Contains(w.Body.String(), "Ship clear tickets") {
		t.Fatal(w.Body.String())
	}
	req(s, "POST", "/api/chat", `{"text":"/done c1"}`)
	s, _ = New(c)
	if !s.data.Commitments[0].Done {
		t.Fatal("completion lost")
	}
}
func TestSecurity(t *testing.T) {
	s, _ := New(Config{DataDir: t.TempDir()})
	for _, tc := range []struct{ host, origin string }{{"evil.example", ""}, {"127.0.0.1:3850", "https://evil.example"}} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/chat", strings.NewReader(`{"text":"/commit Evil"}`))
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
	if len(s.data.Messages) != 0 {
		t.Fatal("unauthorized mutation")
	}
	if _, e := New(Config{DataDir: t.TempDir(), Listen: "0.0.0.0:3850"}); e == nil {
		t.Fatal("public bind allowed")
	}
}
func TestPolicyAndCallbacks(t *testing.T) {
	calls := 0
	s, _ := New(Config{DataDir: t.TempDir(), Check: func(ctx context.Context, obj, act string) error {
		if act == "write" {
			return errors.New("denied")
		}
		return nil
	}, Search: func(ctx context.Context, q, scope string) (any, error) { calls++; return []string{q, scope}, nil }})
	if w := req(s, "POST", "/api/chat", `{"text":"hello"}`); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := req(s, "GET", "/api/search?q=hello&scope=wiki", "")
	var v []string
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || calls != 1 || v[1] != "wiki" {
		t.Fatal(w.Body.String())
	}
	if w := req(s, http.MethodGet, "/", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestFocusDeferAndContext(t *testing.T) {
	s, _ := New(Config{DataDir: t.TempDir()})
	if _, err := s.AddCommitment(context.Background(), "Unconfirmed", false); err == nil {
		t.Fatal("accepted unconfirmed commitment")
	}
	c, err := s.AddCommitment(context.Background(), "First", true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddCommitment(context.Background(), "Second", true)
	if err != nil {
		t.Fatal(err)
	}
	req(s, "POST", "/api/chat", `{"text":"/focus `+second.ID+`"}`)
	if s.data.Commitments[0].ID != second.ID {
		t.Fatal("focus not reordered")
	}
	req(s, "POST", "/api/chat", `{"text":"/defer `+second.ID+`"}`)
	w := req(s, "POST", "/api/chat", `{"text":"status"}`)
	if !strings.Contains(w.Body.String(), "Focus: First") {
		t.Fatal(w.Body.String())
	}
	ctx := s.Context()
	if !strings.Contains(ctx, c.ID+" [active] due=: First") || !strings.Contains(ctx, second.ID+" [deferred] due=: Second") {
		t.Fatal(ctx)
	}
}

func TestStatusMetadata(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), Metadata: map[string]string{"model": "local-small"}})
	if err != nil {
		t.Fatal(err)
	}
	w := req(s, "GET", "/api/status", "")
	var v struct {
		Metadata      map[string]string `json:"metadata"`
		ChatAvailable bool              `json:"chat_available"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Metadata["model"] != "local-small" || v.ChatAvailable {
		t.Fatal(w.Body.String())
	}
}

func TestScopedStatusCallback(t *testing.T) {
	scope := "unset"
	s, err := New(Config{DataDir: t.TempDir(), Status: func(ctx context.Context, q string) string { scope = q; return "Cached tasks checked today." }})
	if err != nil {
		t.Fatal(err)
	}
	w := req(s, "POST", "/api/chat", `{"text":"status ffai"}`)
	if scope != "ffai" || !strings.Contains(w.Body.String(), "Cached tasks checked today.") {
		t.Fatal(scope, w.Body.String())
	}
	req(s, "POST", "/api/chat", `{"text":"status"}`)
	if scope != "" {
		t.Fatal(scope)
	}
}
