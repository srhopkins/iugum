package agentplatform

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/contract"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestConversationHandoffNativeCreateResumeAndStop(t *testing.T) {
	cwd := t.TempDir()
	const id = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	created := 0
	requests := make(chan string, 10)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path + "?" + r.URL.RawQuery
		switch r.URL.Path {
		case "/api/conversations":
			if r.Method == "POST" {
				created++
				json.NewEncoder(w).Encode(map[string]string{"id": id})
			} else {
				json.NewEncoder(w).Encode([]map[string]string{{"id": id, "title": "native"}})
			}
		case "/api/messages":
			json.NewEncoder(w).Encode([]contract.AgentSessionMessage{{Role: "assistant", Text: "native history"}})
		case "/api/chat":
			io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
		case "/api/approvals":
			json.NewEncoder(w).Encode([]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	p := NativeHTTPProvider{BaseURL: backend.URL, Cwd: cwd}
	o := &managed{done: context.Background(), requests: map[string]*approval{}}
	ref, client, err := p.Create(context.Background(), contract.AgentSessionCreate{Provider: "Iugum", Cwd: cwd}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if ref.ID != id || created != 1 {
		t.Fatal("wrong native creation")
	}
	history, err := client.History(context.Background())
	if err != nil || len(history) != 1 {
		t.Fatalf("native history %v %v", history, err)
	}
	done := make(chan error, 1)
	go func() { done <- client.Prompt(context.Background(), "work") }()
	for {
		if path := <-requests; path == "/api/chat?conversation="+id {
			break
		}
	}
	if err := client.Interrupt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("cancel falsely confirmed completion")
	}
	if _, err := p.Open(context.Background(), contract.AgentSessionRef{Provider: "Iugum", ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Cwd: cwd}, o); err == nil {
		t.Fatal("missing resume created replacement")
	}
	if created != 1 {
		t.Fatal("resume created session")
	}
}

func TestConversationHandoffNativeApprovalAndCrossProviderBackground(t *testing.T) {
	cwd := t.TempDir()
	const nativeSession = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	approved := make(chan string, 1)
	var mu sync.Mutex
	history := []contract.AgentSessionMessage{}
	pending := true
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations" && r.URL.Query().Get("conversation") != nativeSession {
			t.Error("native target changed")
			http.Error(w, "wrong session", 409)
			return
		}
		switch r.URL.Path {
		case "/api/conversations":
			json.NewEncoder(w).Encode([]map[string]string{{"id": nativeSession}})
		case "/api/messages":
			mu.Lock()
			defer mu.Unlock()
			json.NewEncoder(w).Encode(history)
		case "/api/approvals":
			mu.Lock()
			defer mu.Unlock()
			rows := []map[string]any{}
			if pending {
				rows = append(rows, map[string]any{"id": "approval-fixture", "digest": "original-digest", "status": "pending", "title": "fixture action", "action": "test"})
			}
			json.NewEncoder(w).Encode(rows)
		case "/api/approvals/approval-fixture":
			var q struct{ Decision, Digest string }
			json.NewDecoder(r.Body).Decode(&q)
			if q.Digest != "original-digest" {
				t.Error("approval digest changed")
			}
			mu.Lock()
			pending = false
			mu.Unlock()
			approved <- q.Decision
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/api/chat":
			io.Copy(io.Discard, r.Body)
			started <- struct{}{}
			select {
			case <-release:
				mu.Lock()
				history = append(history, contract.AgentSessionMessage{Role: "assistant", Text: "native final answer"})
				mu.Unlock()
				json.NewEncoder(w).Encode(map[string]string{"reply": "native final answer"})
			case <-r.Context().Done():
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	other := &handoffProvider{name: "Cursor", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{NativeHTTPProvider{BaseURL: backend.URL, Cwd: cwd}, other}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := contract.AgentSessionRef{Provider: "Iugum", ID: nativeSession, Cwd: cwd}
	c := handoffCreate(t, s, ref)
	path := "/api/conversations/" + c.ID
	if w := callService(s, "POST", path+"/chat", map[string]any{"revision": c.Revision, "session": ref, "text": "work", "commandId": testID}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	<-started
	var approvals []struct{ ID, Digest string }
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w := callService(s, "GET", "/api/agents/iugum-session/approvals?conversation="+nativeSession, nil)
		json.Unmarshal(w.Body.Bytes(), &approvals)
		if len(approvals) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(approvals) != 1 {
		t.Fatal("native approval unavailable")
	}
	if w := callService(s, "POST", path+"/switch", map[string]any{"revision": c.Revision, "session": contract.AgentSessionRef{Provider: "Cursor", ID: otherID, Cwd: cwd}}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := callService(s, "POST", "/api/agents/iugum-session/approve?conversation="+nativeSession, map[string]any{"id": approvals[0].ID, "digest": approvals[0].Digest, "result": map[string]string{"decision": "decline"}}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case decision := <-approved:
		if decision != "reject" {
			t.Fatal("native decision changed")
		}
	case <-time.After(time.Second):
		t.Fatal("native decision missed original request")
	}
	close(release)
	c = handoffAwait(t, s, c.ID, "complete")
	if c.Selected.Provider != "Cursor" || len(c.Messages) != 2 || c.Messages[1].Text != "native final answer" {
		t.Fatalf("native background lost: %+v", c)
	}
}
