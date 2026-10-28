package agentplatform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaudeProviderRequiresExplicitConfiguration(t *testing.T) {
	p := ClaudeProvider{}
	_, err := p.Open(context.Background(), contract.AgentSessionRef{Provider: "Claude", ID: "12345678-1234-1234-1234-123456789abc", Cwd: t.TempDir()}, nil)
	if err == nil {
		t.Fatal("missing manager/account/config must fail closed")
	}
}

// The helper is a protocol peer, never a real Claude account or model.
func TestClaudeProviderHelper(t *testing.T) {
	helper := false
	id := ""
	for i, a := range os.Args {
		if a == "--print" {
			helper = true
		}
		if (a == "--resume" || a == "--session-id") && i+1 < len(os.Args) {
			id = os.Args[i+1]
		}
	}
	if !helper {
		return
	}
	emit := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var f map[string]json.RawMessage
		_ = json.Unmarshal(scanner.Bytes(), &f)
		var typ string
		_ = json.Unmarshal(f["type"], &typ)
		switch typ {
		case "control_request":
			var request struct {
				Subtype string `json:"subtype"`
			}
			_ = json.Unmarshal(f["request"], &request)
			cwd, _ := os.Getwd()
			if request.Subtype == "interrupt" {
				if _, err := os.Stat(filepath.Join(cwd, "no-interrupt-ack")); err == nil {
					continue
				}
			}
			var rid string
			_ = json.Unmarshal(f["request_id"], &rid)
			emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": rid}})
		case "user":
			var msg struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal(f["message"], &msg)
			if strings.Contains(msg.Content, "disconnect") {
				os.Exit(0)
			}
			actual := id
			if strings.Contains(msg.Content, "replace") {
				actual = "foreign"
			}
			emit(map[string]any{"type": "system", "subtype": "init", "session_id": actual})
			emit(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_delta", "delta": map[string]string{"type": "text_delta", "text": "hello"}}})
			emit(map[string]any{"type": "tool_progress", "tool_name": "Read"})
			emit(map[string]any{"type": "control_request", "request_id": "approval-1", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]string{"command": "pwd"}}})
		case "control_response":
			var response struct {
				Subtype  string `json:"subtype"`
				Response struct {
					Behavior string `json:"behavior"`
				} `json:"response"`
			}
			_ = json.Unmarshal(f["response"], &response)
			if response.Subtype == "success" && response.Response.Behavior != "deny" && response.Response.Behavior != "allow" {
				os.Exit(9)
			}
			emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]string{"type": "text", "text": "hello"}}}})
			emit(map[string]any{"type": "result", "session_id": id, "result": "hello"})
		}
	}
	os.Exit(0)
}

type claudeGate struct{}

func (claudeGate) Enforce(context.Context, contract.Request) error { return nil }

type claudeObserver struct {
	mu       sync.Mutex
	events   []string
	requests int
	answer   json.RawMessage
}

func (o *claudeObserver) Event(k, v string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, k+":"+v)
}
func (o *claudeObserver) Request(_ context.Context, r contract.AgentSessionRequest) (json.RawMessage, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests++
	if r.ID != "approval-1" || r.Method != "can_use_tool" {
		return nil, fmt.Errorf("wrong permission identity")
	}
	return o.answer, nil
}
func claudeSetup(t *testing.T, observer contract.AgentSessionObserver) contract.AgentSession {
	t.Helper()
	dir := t.TempDir()
	binary, _ := os.Executable()
	script := filepath.Join(dir, "claude")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestClaudeProviderHelper -- \"$@\"\n", binary)), 0700); err != nil {
		t.Fatal(err)
	}
	manager, err := agentcontrol.New(filepath.Join(dir, "state"), claudeGate{})
	if err != nil {
		t.Fatal(err)
	}
	manager.Executable = script
	manager.VerifyAccount = func(context.Context, string, agentcontrol.StartRequest) error { return nil }
	ref := contract.AgentSessionRef{Provider: "Claude", ID: "12345678-1234-1234-1234-123456789abc", Cwd: dir}
	path := filepath.Join(dir, "projects", claudePathEscape.ReplaceAllString(dir, "-"))
	_ = os.MkdirAll(path, 0700)
	row, _ := json.Marshal(map[string]any{"type": "user", "sessionId": ref.ID, "cwd": dir, "message": map[string]string{"role": "user", "content": "old history"}})
	_ = os.WriteFile(filepath.Join(path, ref.ID+".jsonl"), append(row, '\n'), 0600)
	p := ClaudeProvider{Manager: manager, Actor: "test", Account: "test@example.com", ConfigDir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	s, err := p.Open(ctx, ref, observer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func TestClaudeProviderExactResumeHistoryProgressApprovalAndInterrupt(t *testing.T) {
	observer := &claudeObserver{answer: json.RawMessage(`{"behavior":"deny","message":"No"}`)}
	s := claudeSetup(t, observer)
	history, err := s.History(context.Background())
	if err != nil || len(history) != 1 || history[0].Text != "old history" {
		t.Fatal(history, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = s.Prompt(ctx, "continue"); err != nil {
		t.Fatal(err)
	}
	if err = s.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.requests != 1 {
		t.Fatal("approval missing")
	}
	events := strings.Join(observer.events, "\n")
	if !strings.Contains(events, "text:hello") || !strings.Contains(events, "progress:") {
		t.Fatal(events)
	}
}
func TestClaudeProviderDisconnectNoReplay(t *testing.T) {
	s := claudeSetup(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Prompt(ctx, "disconnect"); err == nil {
		t.Fatal("disconnect must fail")
	}
	if err := s.Prompt(ctx, "no automatic replay"); err == nil {
		t.Fatal("dead process reused")
	}
}
func TestClaudeProviderRejectsSessionReplacement(t *testing.T) {
	s := claudeSetup(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Prompt(ctx, "replace"); err == nil {
		t.Fatal("replacement accepted")
	}
}
func TestClaudeProviderNeverInfersApproval(t *testing.T) {
	s := claudeSetup(t, &claudeObserver{answer: json.RawMessage(`{"text":"yes"}`)})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Prompt(ctx, "ordinary yes is not permission"); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeProviderExplicitAllow(t *testing.T) {
	observer := &claudeObserver{answer: json.RawMessage(`{"behavior":"allow","updatedInput":{"command":"pwd"}}`)}
	s := claudeSetup(t, observer)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Prompt(ctx, "explicit allow"); err != nil {
		t.Fatal(err)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.requests != 1 {
		t.Fatal("missing explicit approval request")
	}
}

func TestClaudeProviderInterruptRequiresAcknowledgement(t *testing.T) {
	s := claudeSetup(t, nil)
	dir := s.(*claudeSession).ref.Cwd
	if err := os.WriteFile(filepath.Join(dir, "no-interrupt-ack"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := s.Interrupt(ctx); err == nil {
		t.Fatal("stop reported success without control acknowledgement")
	}
}

func TestSessionControlsClaudeCreateIdentityAndDefault(t *testing.T) {
	dir := t.TempDir()
	binary, _ := os.Executable()
	script := filepath.Join(dir, "claude")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestClaudeProviderHelper -- \"$@\"\n", binary)), 0700); err != nil {
		t.Fatal(err)
	}
	manager, err := agentcontrol.New(filepath.Join(dir, "state"), claudeGate{})
	if err != nil {
		t.Fatal(err)
	}
	manager.Executable = script
	manager.VerifyAccount = func(context.Context, string, agentcontrol.StartRequest) error { return nil }
	p := ClaudeProvider{Manager: manager, Actor: "test", Account: "test@example.com", ConfigDir: dir, Model: "configured-model"}
	o := &claudeObserver{answer: json.RawMessage(`{"behavior":"deny","message":"No"}`)}
	ref, c, err := p.Create(context.Background(), contract.AgentSessionCreate{Provider: "Claude", Cwd: dir}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !uuid.MatchString(ref.ID) {
		t.Fatal(ref)
	}
	if err = c.Prompt(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	st, err := manager.Snapshot(context.Background(), "test", ref.ID)
	if err != nil || st.SessionID != ref.ID || st.Request.NewSessionID != ref.ID || st.Request.ResumeSessionID != "" {
		t.Fatalf("%+v %v", st, err)
	}
	models, err := c.(contract.AgentSessionModelController).Models(context.Background())
	if err != nil || models.Current != "configured-model" || models.CanSelect {
		t.Fatalf("%+v %v", models, err)
	}
}
