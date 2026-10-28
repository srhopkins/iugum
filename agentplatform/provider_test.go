package agentplatform

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/contract"
	"sync"
	"testing"
	"time"
)

type protocolObserver struct {
	mu       sync.Mutex
	texts    []string
	requests []contract.AgentSessionRequest
}

func (o *protocolObserver) Event(k, v string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.texts = append(o.texts, k+":"+v)
}
func (o *protocolObserver) Request(_ context.Context, r contract.AgentSessionRequest) (json.RawMessage, error) {
	o.requests = append(o.requests, r)
	return json.RawMessage(`{"decision":"decline"}`), nil
}
func openProtocolProvider(t *testing.T, name, mode string) (contract.AgentSession, *protocolObserver) {
	t.Helper()
	o := &protocolObserver{}
	s, e := (Provider{Provider: name, Binary: helperBinary(t, mode)}).Open(context.Background(), contract.AgentSessionRef{Provider: name, ID: testSessionID, Cwd: t.TempDir()}, o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, o
}
func TestCodexProviderResumeHistoryAndStaleCompletion(t *testing.T) {
	s, _ := openProtocolProvider(t, "Codex", "")
	h, e := s.History(context.Background())
	if e != nil || len(h) != 1 || h[0].Text != "history" {
		t.Fatalf("history=%v err=%v", h, e)
	}
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	if e = s.Prompt(ctx, "hello"); e != nil {
		t.Fatal(e)
	}
}
func TestCursorProviderHistoryAndProgress(t *testing.T) {
	s, o := openProtocolProvider(t, "Cursor", "")
	h, e := s.History(context.Background())
	if e != nil || len(h) != 1 || h[0].Text != "history" {
		t.Fatalf("history=%v err=%v", h, e)
	}
	if e = s.Prompt(context.Background(), "hello"); e != nil {
		t.Fatal(e)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.texts) != 1 || o.texts[0] != "text:answer" {
		t.Fatalf("events %v", o.texts)
	}
}
func TestCodexProviderFailedResumeNoReplacement(t *testing.T) {
	s, _ := openProtocolProvider(t, "Codex", "reject")
	if e := s.Prompt(context.Background(), "hello"); e == nil {
		t.Fatal("resume failure accepted")
	}
}
func TestCursorProviderFailedLoadNoReplacement(t *testing.T) {
	_, e := (Provider{Provider: "Cursor", Binary: helperBinary(t, "reject")}).Open(context.Background(), contract.AgentSessionRef{Provider: "Cursor", ID: testSessionID, Cwd: t.TempDir()}, nil)
	if e == nil {
		t.Fatal("load failure accepted")
	}
}
func TestCodexProviderRequestsFailClosed(t *testing.T) {
	testRequests(t, "Codex", []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/tool/requestUserInput"})
}
func TestCursorProviderRequestsFailClosed(t *testing.T) {
	testRequests(t, "Cursor", []string{"session/request_permission", "cursor/ask_question", "cursor/create_plan"})
}
func testRequests(t *testing.T, name string, methods []string) {
	o := &protocolObserver{}
	s := &session{ref: contract.AgentSessionRef{Provider: name, ID: testSessionID}, observer: o, lifetime: context.Background()}
	key := "sessionId"
	if name == "Codex" {
		key = "threadId"
	}
	for _, m := range methods {
		p := mustJSON(map[string]any{key: testSessionID, "options": []any{map[string]string{"optionId": "deny", "kind": "reject_once"}}, "questions": []any{map[string]string{"id": "q", "prompt": "Choose"}}, "plan": "proposed"})
		r, e := s.request(frame{ID: json.RawMessage(`"approval-7"`), Method: m, Params: p})
		if e != nil || string(r) != `{"decision":"decline"}` {
			t.Fatalf("%s: %s %v", m, r, e)
		}
		got := o.requests[len(o.requests)-1]
		if got.ID != `"approval-7"` || string(got.Params) != string(p) {
			t.Fatal("request identity or choices changed")
		}
	}
	for _, p := range []json.RawMessage{mustJSON(map[string]string{key: "foreign"}), mustJSON(map[string]string{})} {
		if _, e := s.request(frame{ID: json.RawMessage(`9`), Method: methods[0], Params: p}); e == nil {
			t.Fatal("unscoped approval accepted")
		}
	}
	if _, e := s.request(frame{Method: "fs/write_text_file", Params: mustJSON(map[string]string{key: testSessionID})}); e == nil {
		t.Fatal("unsupported request accepted")
	}
}

func TestCodexProviderInterruptAcknowledgement(t *testing.T) {
	api, _ := openProtocolProvider(t, "Codex", "interrupt")
	s := api.(*session)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Prompt(ctx, "work") }()
	for {
		s.mu.Lock()
		id := s.turnID
		s.mu.Unlock()
		if id != "" {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("prompt ended before interrupt: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	if err := s.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func TestCodexProviderDisconnectOutcomeUncertain(t *testing.T) {
	s, _ := openProtocolProvider(t, "Codex", "disconnect")
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	if e := s.Prompt(ctx, "one delivery"); e == nil {
		t.Fatal("lost connection reported success")
	}
	if e := s.Prompt(ctx, "no reconnect"); e == nil {
		t.Fatal("disconnected session accepted prompt")
	}
}
func TestCursorProviderDisconnectNoReplay(t *testing.T) {
	s, _ := openProtocolProvider(t, "Cursor", "disconnect")
	if e := s.Prompt(context.Background(), "one delivery"); e == nil {
		t.Fatal("lost connection reported success")
	}
	if e := s.Prompt(context.Background(), "no reconnect"); e == nil {
		t.Fatal("disconnected session accepted prompt")
	}
}

// Cursor extension requests omit sessionId in the official single-session ACP connection.
func TestCursorProviderCustomQuestionsWithoutSessionID(t *testing.T) {
	o := &protocolObserver{}
	s := &session{ref: contract.AgentSessionRef{Provider: "Cursor", ID: testSessionID}, observer: o, lifetime: context.Background()}
	for _, m := range []string{"cursor/ask_question", "cursor/create_plan"} {
		p := mustJSON(map[string]any{"toolCallId": "question-1", "title": "Question", "questions": []any{map[string]any{"id": "scope", "prompt": "Which scope?", "options": []any{map[string]string{"id": "workspace", "label": "Workspace"}}}}, "plan": "proposed"})
		if _, e := s.request(frame{ID: json.RawMessage(`"custom-1"`), Method: m, Params: p}); e != nil {
			t.Fatal(e)
		}
		if got := o.requests[len(o.requests)-1]; string(got.Params) != string(p) {
			t.Fatal("custom request changed")
		}
	}
}

func TestCodexProviderProgressAndForeignEvents(t *testing.T) {
	o := &protocolObserver{}
	s := &session{ref: contract.AgentSessionRef{Provider: "Codex", ID: testSessionID}, observer: o}
	for _, id := range []string{"foreign", testSessionID} {
		s.notify(frame{Method: "item/agentMessage/delta", Params: mustJSON(map[string]string{"threadId": id, "delta": "answer"})})
	}
	s.notify(frame{Method: "item/started", Params: mustJSON(map[string]any{"threadId": testSessionID, "item": map[string]string{"type": "commandExecution", "command": "go test"}})})
	if len(o.texts) != 2 || o.texts[0] != "text:answer" || o.texts[1] != "tool:commandExecution go test" {
		t.Fatalf("events: %v", o.texts)
	}
}
