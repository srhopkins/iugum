package agentdesk

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTurnFailurePersistsApartFromAssistantText(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), Chat: func(context.Context, string) (string, error) {
		return "Partial answer.", NewRuntimeTurnError(errors.New(`MCP private-token {"payload":"secret"}`), "Tool loop: result=failed steps=2; step=1 shell=exit_7")
	}}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	response := req(s, "POST", "/api/chat", `{"text":"hello"}`)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var m Message
	if err := json.Unmarshal(response.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Role != "error" || m.Failure == nil || !strings.Contains(m.Failure.Details, "shell=exit_7") {
		t.Fatalf("bad failure: %+v", m)
	}
	for _, unsafe := range []string{"private-token", "payload", "secret"} {
		if strings.Contains(response.Body.String(), unsafe) {
			t.Fatal("raw diagnostic exposed")
		}
	}
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.data.Messages) != 3 || s.data.Messages[1].Role != "assistant" || s.data.Messages[1].Text != "Partial answer." || s.data.Messages[2].Role != "error" {
		t.Fatalf("history lost: %+v", s.data.Messages)
	}
	cfg.Chat = func(context.Context, string) (string, error) { return "Recovered.", nil }
	s, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	response = req(s, "POST", "/api/chat", `{"text":"retry"}`)
	if !strings.Contains(response.Body.String(), `"role":"assistant"`) || !strings.Contains(response.Body.String(), "Recovered.") {
		t.Fatal(response.Body.String())
	}
}

func TestTurnFailureNeverPublishesArbitraryDiagnostics(t *testing.T) {
	for _, err := range []error{errors.New("Bearer secret /private/data"), NewRuntimeTurnError(errors.New("private"), "Tool loop: result=failed steps=1; secret=private"), NewRuntimeTurnError(errors.New("private"), strings.Repeat("x", 5000))} {
		f := turnFailure(err)
		if strings.Contains(f.Details, "secret") || strings.Contains(f.Details, "private") || len(f.Details) > 4096 {
			t.Fatalf("unsafe failure: %+v", f)
		}
	}
	if turnFailure(context.Canceled).Code != "turn_canceled" || turnFailure(context.DeadlineExceeded).Code != "turn_timeout" {
		t.Fatal("cancellation classification lost")
	}
}

func TestLegacyRuntimeFailureViewLeavesStorageUntouched(t *testing.T) {
	m := Message{Role: "assistant", Text: `I couldn't complete that request: private payload (Tool loop: result=failed steps=2)`}
	v := viewMessage(m)
	if v.Role != "error" || v.Failure == nil || strings.Contains(v.Text, "private") || strings.Contains(v.HTML, "private") {
		t.Fatalf("legacy failure exposed: %+v", v)
	}
	if !strings.Contains(m.Text, "private") {
		t.Fatal("legacy source mutated")
	}
	normal := viewMessage(Message{Role: "assistant", Text: "I couldn't complete that request: please specify a file."})
	if normal.Role != "assistant" {
		t.Fatal("ordinary model text reclassified")
	}
}
