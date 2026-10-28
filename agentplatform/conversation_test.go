package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConversationHandoffCreateSwitchAndReopen(t *testing.T) {
	s, _ := testServer(t)
	cwd := t.TempDir()
	a := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd, Title: "first"}
	b := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd, Title: "second"}
	w := callService(s, "POST", "/api/conversations", map[string]any{"session": a, "importHistory": true})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" || len(c.Messages) != 1 || c.Messages[0].Provenance != "imported" {
		t.Fatalf("missing explicit import: %+v", c)
	}
	w = callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": b})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var switched contract.AgentConversation
	json.Unmarshal(w.Body.Bytes(), &switched)
	if switched.Selected.ID != b.ID || len(switched.Links) != 2 || len(switched.Messages) != 1 {
		t.Fatalf("switch lost history: %+v", switched)
	}
	s.Close()
	reopened, err := New(context.Background(), s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	w = callService(reopened, "GET", "/api/conversations/"+c.ID, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "saved history") {
		t.Fatalf("reopen: %s", w.Body.String())
	}
}
func TestConversationHandoffContextBounds(t *testing.T) {
	messages := []contract.AgentConversationMessage{}
	for i := 0; i < 40; i++ {
		messages = append(messages, contract.AgentConversationMessage{Role: "assistant", Text: strings.Repeat("世", 2000)})
	}
	prompt := conversationPrompt(messages, "current request")
	if len(prompt) > 34000 || !strings.Contains(prompt, "untrusted") || !strings.HasSuffix(prompt, "current request") {
		t.Fatalf("unsafe/unbounded context: %d", len(prompt))
	}
}

type handoffProvider struct {
	testProvider
	name     string
	sessions map[string]*handoffSession
}

func (p *handoffProvider) Name() string { return p.name }
func (p *handoffProvider) Open(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (contract.AgentSession, error) {
	c := &handoffSession{history: []contract.AgentSessionMessage{}, started: make(chan string, 1), finish: make(chan struct{}), observer: o}
	p.sessions[ref.ID] = c
	return c, nil
}

type handoffSession struct {
	mu       sync.Mutex
	history  []contract.AgentSessionMessage
	started  chan string
	finish   chan struct{}
	observer contract.AgentSessionObserver
	failure  bool
}

func (c *handoffSession) History(context.Context) ([]contract.AgentSessionMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]contract.AgentSessionMessage{}, c.history...), nil
}
func (c *handoffSession) Prompt(ctx context.Context, text string) error {
	c.started <- text
	select {
	case <-c.finish:
	case <-ctx.Done():
		return ctx.Err()
	}
	if c.failure {
		return errors.New("lost delivery")
	}
	c.mu.Lock()
	c.history = append(c.history, contract.AgentSessionMessage{Role: "user", Text: text}, contract.AgentSessionMessage{Role: "assistant", Text: "provider answer"})
	c.mu.Unlock()
	return nil
}
func (c *handoffSession) Interrupt(context.Context) error { return nil }
func (c *handoffSession) Close() error                    { return nil }
func handoffCreate(t *testing.T, s *Server, ref contract.AgentSessionRef) contract.AgentConversation {
	t.Helper()
	w := callService(s, "POST", "/api/conversations", map[string]any{"session": ref})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	json.Unmarshal(w.Body.Bytes(), &c)
	return c
}
func handoffAwait(t *testing.T, s *Server, id string, status string) contract.AgentConversation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		w := callService(s, "GET", "/api/conversations/"+id, nil)
		var c contract.AgentConversation
		json.Unmarshal(w.Body.Bytes(), &c)
		if len(c.Commands) > 0 && c.Commands[len(c.Commands)-1].Status == status {
			return c
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("command never became " + status)
	return contract.AgentConversation{}
}
func TestConversationHandoffBackgroundExactTargetContextAndDuplicate(t *testing.T) {
	a := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	b := &handoffProvider{name: "Cursor", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd}
	other := contract.AgentSessionRef{Provider: "Cursor", ID: otherID, Cwd: cwd}
	c := handoffCreate(t, s, ref)
	path := "/api/conversations/" + c.ID
	send := map[string]any{"revision": c.Revision, "session": ref, "text": "remember the blue widget", "commandId": testID}
	if w := callService(s, "POST", path+"/chat", send); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-a.sessions[testID].started:
	case <-time.After(time.Second):
		t.Fatal("not started")
	}
	if w := callService(s, "POST", path+"/chat", send); w.Code == 200 {
		t.Fatal("duplicate accepted")
	}
	if w := callService(s, "POST", path+"/switch", map[string]any{"revision": c.Revision, "session": other}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	close(a.sessions[testID].finish)
	c = handoffAwait(t, s, c.ID, "complete")
	if c.Selected.Provider != "Cursor" || len(c.Messages) != 2 || c.Messages[1].Provider != "Codex" {
		t.Fatalf("background redirected/lost: %+v", c)
	}
	if w := callService(s, "POST", path+"/chat", map[string]any{"revision": 1, "session": ref, "text": "stale", "commandId": otherID}); w.Code == 200 {
		t.Fatal("stale writer accepted")
	}
	if w := callService(s, "POST", path+"/chat", map[string]any{"revision": c.Revision, "session": other, "text": "continue", "commandId": otherID}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	prompt := <-b.sessions[otherID].started
	if !strings.Contains(prompt, "blue widget") || !strings.Contains(prompt, "provider answer") || !strings.Contains(prompt, "untrusted") {
		t.Fatalf("context missing: %s", prompt)
	}
	close(b.sessions[otherID].finish)
	c = handoffAwait(t, s, c.ID, "complete")
	if len(c.Messages) != 4 {
		t.Fatalf("canonical history duplicated or missing: %+v", c.Messages)
	}
}
func TestConversationHandoffUncertainRestartNeverReplays(t *testing.T) {
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	cfg := Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}}
	s, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	c := handoffCreate(t, s, ref)
	path := "/api/conversations/" + c.ID
	q := map[string]any{"revision": c.Revision, "session": ref, "text": "one prompt", "commandId": testID}
	if w := callService(s, "POST", path+"/chat", q); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	<-p.sessions[testID].started
	s.Close()
	s, err = New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c = handoffAwait(t, s, c.ID, "uncertain")
	if c.Commands[0].Session.ID != ref.ID {
		t.Fatal("lost original target")
	}
	if w := callService(s, "POST", path+"/chat", q); w.Code == 200 {
		t.Fatal("uncertain command replayed")
	}
	select {
	case <-p.sessions[testID].started:
		t.Fatal("reopen replayed native prompt")
	default:
	}
}

func TestConversationHandoffApprovalsRemainAtOriginalSession(t *testing.T) {
	s, p := testServer(t)
	cwd := t.TempDir()
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd}
	other := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd}
	c := handoffCreate(t, s, ref)
	path := "/api/conversations/" + c.ID
	if w := callService(s, "POST", path+"/chat", map[string]any{"revision": c.Revision, "session": ref, "text": "work", "commandId": testID}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	awaitStarted(t, p.clients[testID])
	result := make(chan error, 1)
	go func() {
		_, err := p.clients[testID].observer.Request(context.Background(), contract.AgentSessionRequest{Method: "item/commandExecution/requestApproval"})
		result <- err
	}()
	var approvals []struct {
		ID     string `json:"id"`
		Digest string `json:"digest"`
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		w := callService(s, "GET", "/api/agents/codex-session/approvals?conversation="+testID, nil)
		json.Unmarshal(w.Body.Bytes(), &approvals)
		if len(approvals) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(approvals) != 1 {
		t.Fatal("approval not visible")
	}
	if w := callService(s, "POST", path+"/switch", map[string]any{"revision": c.Revision, "session": other}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	answer := map[string]any{"id": approvals[0].ID, "digest": approvals[0].Digest, "result": map[string]string{"decision": "decline"}}
	if w := callService(s, "POST", "/api/agents/codex-session/approve?conversation="+otherID, answer); w.Code == 200 {
		t.Fatal("approval moved to selected session")
	}
	if w := callService(s, "POST", "/api/agents/codex-session/approve?conversation="+testID, answer); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("original request not resolved")
	}
	if w := callService(s, "POST", "/api/agents/codex-session/interrupt?conversation="+testID, map[string]any{}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case <-p.clients[testID].interrupted:
	default:
		t.Fatal("stop missed original target")
	}
	select {
	case <-p.clients[otherID].interrupted:
		t.Fatal("stop redirected")
	default:
	}
	close(p.clients[testID].finish)
	handoffAwait(t, s, c.ID, "complete")
}
func TestConversationHandoffPolicyAndLegacyBypass(t *testing.T) {
	s, _ := testServer(t)
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	c := handoffCreate(t, s, ref)
	if w := callService(s, "POST", "/api/agents/codex-session/chat?conversation="+testID, map[string]string{"text": "bypass", "commandId": testID}); w.Code == 200 {
		t.Fatal("linked session bypassed canonical history")
	}
	s.cfg.Check = func(_ context.Context, resource, action string) error {
		if resource == "agent/conversations/"+c.ID {
			return errors.New("denied")
		}
		return nil
	}
	if w := callService(s, "GET", "/api/conversations/"+c.ID, nil); w.Code == 200 {
		t.Fatal("policy denied history leaked")
	}
	if w := callService(s, "POST", "/api/conversations/"+c.ID+"/chat", map[string]any{"revision": c.Revision, "session": ref, "text": "denied", "commandId": testID}); w.Code == 200 {
		t.Fatal("policy denied send accepted")
	}
	w := callService(s, "GET", "/api/conversations", nil)
	if strings.Contains(w.Body.String(), c.ID) {
		t.Fatal("list leaked denied conversation")
	}
}

func TestConversationHandoffNativePayloadBoundAndCommitment(t *testing.T) {
	history := []contract.AgentConversationMessage{{Role: "assistant", Text: strings.Repeat("history ", 4000)}}
	prompt, err := nativeConversationPrompt(history, strings.Repeat("x", 50000))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"text": prompt})
	if len(body) > 64<<10 {
		t.Fatal("native request exceeds listener bound")
	}
	if _, err := nativeConversationPrompt(history, strings.Repeat("x", 70000)); err == nil {
		t.Fatal("oversized native message accepted")
	}
	prompt, err = nativeConversationPrompt(history, "/commit test commitment")
	if err != nil || prompt != "/commit test commitment" {
		t.Fatal("native commitment command lost")
	}
}

func TestConversationHandoffPersistenceFailureVisibleAsUncertain(t *testing.T) {
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	c := handoffCreate(t, s, ref)
	if w := callService(s, "POST", "/api/conversations/"+c.ID+"/chat", map[string]any{"revision": c.Revision, "session": ref, "text": "work", "commandId": testID}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	<-p.sessions[testID].started
	path := filepath.Join(s.cfg.DataDir, "conversations.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	close(p.sessions[testID].finish)
	c = handoffAwait(t, s, c.ID, "uncertain")
	if !strings.Contains(c.Commands[0].Error, "persistence") {
		t.Fatal("persistence failure hidden")
	}
}
