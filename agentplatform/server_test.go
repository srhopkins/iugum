package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

const testID = "11111111-1111-4111-8111-111111111111"
const otherID = "22222222-2222-4222-8222-222222222222"

type testProvider struct {
	mu             sync.Mutex
	clients        map[string]*testSession
	openErr        error
	opening        chan struct{}
	release        chan struct{}
	approvalOnOpen bool
}

func (p *testProvider) Name() string { return "Codex" }
func (p *testProvider) Capabilities() contract.AgentSessionCapabilities {
	return contract.AgentSessionCapabilities{Resume: true, Interrupt: true}
}
func (p *testProvider) Open(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (contract.AgentSession, error) {
	if p.opening != nil {
		p.opening <- struct{}{}
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.approvalOnOpen {
		if _, err := o.Request(ctx, contract.AgentSessionRequest{Method: "item/commandExecution/requestApproval"}); err == nil {
			return nil, errors.New("opening approval unexpectedly accepted")
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openErr != nil {
		return nil, p.openErr
	}
	s := &testSession{observer: o, started: make(chan struct{}, 1), finish: make(chan struct{}), interrupted: make(chan struct{}, 1)}
	p.clients[ref.ID] = s
	return s, nil
}

type testSession struct {
	mu          sync.Mutex
	observer    contract.AgentSessionObserver
	started     chan struct{}
	finish      chan struct{}
	interrupted chan struct{}
	closed      bool
	calls       int
}

func (s *testSession) History(context.Context) ([]contract.AgentSessionMessage, error) {
	return []contract.AgentSessionMessage{{Role: "assistant", Text: "saved history"}}, nil
}
func (s *testSession) Prompt(ctx context.Context, text string) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	s.started <- struct{}{}
	select {
	case <-s.finish:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *testSession) Interrupt(context.Context) error { s.interrupted <- struct{}{}; return nil }
func (s *testSession) Close() error                    { s.mu.Lock(); s.closed = true; s.mu.Unlock(); return nil }

func testServer(t *testing.T) (*Server, *testProvider) {
	t.Helper()
	p := &testProvider{clients: map[string]*testSession{}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := New(ctx, Config{DataDir: t.TempDir(), Home: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, p
}
func callService(s *Server, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://127.0.0.1:3853"+path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+s.token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func selectTest(t *testing.T, s *Server, id, cwd string) {
	t.Helper()
	w := callService(s, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: id, Cwd: cwd, Title: id})
	if w.Code != 200 {
		t.Fatalf("select: %d %s", w.Code, w.Body.String())
	}
}
func awaitStarted(t *testing.T, c *testSession) {
	t.Helper()
	select {
	case <-c.started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt never started")
	}
}

func TestSwitchKeepsBackgroundSessionAndTargetsOriginal(t *testing.T) {
	s, p := testServer(t)
	cwd := t.TempDir()
	selectTest(t, s, testID, cwd)
	first := p.clients[testID]
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- callService(s, "POST", "/api/agents/codex-session/chat?conversation="+testID, map[string]string{"text": "work", "commandId": testID})
	}()
	awaitStarted(t, first)
	selectTest(t, s, otherID, cwd)
	first.mu.Lock()
	closed := first.closed
	first.mu.Unlock()
	if closed {
		t.Fatal("switch closed the background session")
	}
	w := callService(s, "GET", "/api/agents/codex-session/status?conversation="+testID, nil)
	if w.Code != 200 {
		t.Fatalf("background status inaccessible: %s", w.Body.String())
	}
	w = callService(s, "POST", "/api/agents/codex-session/interrupt?conversation="+testID, map[string]any{})
	if w.Code != 200 {
		t.Fatalf("background interrupt failed: %s", w.Body.String())
	}
	select {
	case <-first.interrupted:
	default:
		t.Fatal("interrupt did not target original session")
	}
	select {
	case <-p.clients[otherID].interrupted:
		t.Fatal("interrupt targeted new selection")
	default:
	}
	close(first.finish)
	select {
	case w = <-finished:
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("original work did not complete")
	}
	selectTest(t, s, testID, cwd)
	if p.clients[testID] != first {
		t.Fatal("switching back replaced the existing owner")
	}
}

func TestDisconnectedClientDoesNotCancelOrRepeatWork(t *testing.T) {
	s, p := testServer(t)
	selectTest(t, s, testID, t.TempDir())
	c := p.clients[testID]
	ctx, cancel := context.WithCancel(context.Background())
	b, _ := json.Marshal(map[string]string{"text": "once", "commandId": testID})
	r := httptest.NewRequest("POST", "http://127.0.0.1:3853/api/agents/codex-session/chat?conversation="+testID, bytes.NewReader(b)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+s.token)
	done := make(chan struct{})
	go func() { s.Handler().ServeHTTP(httptest.NewRecorder(), r); close(done) }()
	awaitStarted(t, c)
	cancel()
	<-done
	w := callService(s, "POST", "/api/agents/codex-session/chat?conversation="+testID, map[string]string{"text": "once", "commandId": testID})
	if w.Code != 409 {
		t.Fatalf("duplicate returned %d", w.Code)
	}
	c.mu.Lock()
	calls := c.calls
	c.mu.Unlock()
	if calls != 1 {
		t.Fatalf("delivered %d times", calls)
	}
	close(c.finish)
	s.wg.Wait()
	s.mu.Lock()
	status := s.receipts[testID].Status
	s.mu.Unlock()
	if status != "complete" {
		t.Fatalf("background receipt = %s", status)
	}
}

func TestFailedSelectionPreservesOwner(t *testing.T) {
	s, p := testServer(t)
	cwd := t.TempDir()
	selectTest(t, s, testID, cwd)
	first := p.clients[testID]
	p.openErr = errors.New("load rejected")
	w := callService(s, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd})
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = callService(s, "GET", "/api/agents/codex-session/conversation", nil)
	var selected struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &selected); err != nil || selected.ID != testID {
		t.Fatal("failed selection replaced default owner")
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	if first.closed {
		t.Fatal("failed selection closed original")
	}
}

func TestServiceAuthAndPolicy(t *testing.T) {
	s, _ := testServer(t)
	for _, tc := range []struct {
		host, token, origin string
		want                int
	}{
		{"127.0.0.1:3853", "", "", 401}, {"evil.test", s.token, "", 403}, {"127.0.0.1:3853", s.token, "http://evil.test", 403},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+"/api/agents", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
	s.cfg.Check = func(context.Context, string, string) error { return errors.New("denied") }
	if w := callService(s, "GET", "/api/agents", nil); w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestRestartMarksRunningCommandUncertain(t *testing.T) {
	s, _ := testServer(t)
	s.receipts[testID] = receipt{Session: "Codex:" + testID, Status: "running"}
	if err := s.saveReceipts(); err != nil {
		t.Fatal(err)
	}
	next, err := New(context.Background(), s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.receipts[testID].Status != "uncertain" {
		t.Fatal("running command not marked uncertain")
	}
	info, err := os.Stat(next.TokenFile())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("token not private")
	}
}

func TestSwitchIndependentCompletionAndExactRouting(t *testing.T) {
	s, p := testServer(t)
	cwd := t.TempDir()
	selectTest(t, s, testID, cwd)
	first := p.clients[testID]
	selectTest(t, s, otherID, cwd)
	second := p.clients[otherID]
	done := make(chan *httptest.ResponseRecorder, 2)
	for _, id := range []string{testID, otherID} {
		go func(id string) {
			done <- callService(s, "POST", "/api/agents/codex-session/chat?conversation="+id, map[string]string{"text": "work", "commandId": id})
		}(id)
	}
	awaitStarted(t, first)
	awaitStarted(t, second)
	close(second.finish)
	if w := <-done; w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w := callService(s, "GET", "/api/agents/codex-session/status?conversation="+testID, nil)
	var status struct{ Busy bool }
	json.Unmarshal(w.Body.Bytes(), &status)
	if !status.Busy {
		t.Fatal("background turn lost busy state")
	}
	for _, op := range []string{"messages", "events", "approvals", "status"} {
		if w := callService(s, "GET", "/api/agents/codex-session/"+op+"?conversation=unknown", nil); w.Code != 409 {
			t.Fatalf("unknown target %s returned %d", op, w.Code)
		}
	}
	close(first.finish)
	if w := <-done; w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	selectTest(t, s, testID, cwd)
	if w := callService(s, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}); w.Code != 409 {
		t.Fatal("cwd mismatch accepted")
	}
}
func TestSwitchConcurrentOpenAndShutdown(t *testing.T) {
	s, p := testServer(t)
	p.opening = make(chan struct{}, 2)
	p.release = make(chan struct{})
	cwd := t.TempDir()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- callService(s, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	}()
	<-p.opening
	if w := callService(s, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd}); w.Code != 409 {
		t.Fatal("duplicate opening accepted")
	}
	s.Close()
	if w := <-done; w.Code != 409 {
		t.Fatal("shutdown published opening owner")
	}
	if w := callService(s, "GET", "/api/agents/codex-session/status", nil); w.Code != 409 {
		t.Fatal("closed service accepted request")
	}
}
func TestSwitchOpeningApprovalFailsClosed(t *testing.T) {
	s, p := testServer(t)
	p.approvalOnOpen = true
	selectTest(t, s, testID, t.TempDir())
}
func TestSwitchShutdownCancelsBackgroundWork(t *testing.T) {
	s, p := testServer(t)
	selectTest(t, s, testID, t.TempDir())
	c := p.clients[testID]
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- callService(s, "POST", "/api/agents/codex-session/chat?conversation="+testID, map[string]string{"text": "work", "commandId": testID})
	}()
	awaitStarted(t, c)
	s.Close()
	if w := <-done; w.Code != 409 {
		t.Fatal("cancelled turn reported success")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		t.Fatal("owner not closed")
	}
}

func TestSwitchBackgroundApprovalTargetsOwner(t *testing.T) {
	s, p := testServer(t)
	cwd := t.TempDir()
	selectTest(t, s, testID, cwd)
	first := p.clients[testID]
	selectTest(t, s, otherID, cwd)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := first.observer.Request(ctx, contract.AgentSessionRequest{Method: "item/commandExecution/requestApproval"})
		done <- err
	}()
	var pending []approval
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		w := callService(s, "GET", "/api/agents/codex-session/approvals?conversation="+testID, nil)
		if err := json.Unmarshal(w.Body.Bytes(), &pending); err != nil {
			t.Fatal(err)
		}
		if len(pending) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatal("background approval inaccessible")
	}
	answer := map[string]any{"id": pending[0].ID, "digest": pending[0].Digest, "result": map[string]string{"decision": "accept"}}
	if w := callService(s, "POST", "/api/agents/codex-session/approve?conversation="+otherID, answer); w.Code != 409 {
		t.Fatal("approval accepted by wrong owner")
	}
	if w := callService(s, "POST", "/api/agents/codex-session/approve?conversation="+testID, answer); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
