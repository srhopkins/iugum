package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
)

// Exercise real Runtime.Run failure boundaries, then the same native failure
// adapter and durable HTTP chat path used by production. No injected partial
// Chat string: RunResult.Text is empty at these tool/model/step-limit failures.
func TestNativeRuntimeFailurePreservesCurrentTurnPartialAnswer(t *testing.T) {
	for _, failure := range []string{"model", "tool", "step_limit"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 3 {
					http.Error(w, "PRIVATE_PROVIDER_PAYLOAD", http.StatusBadGateway)
					return
				}
				content := []string{"First partial answer.", "Second partial answer."}[calls-1]
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content, "reasoning_content": "PRIVATE_REASONING", "reasoning": "PRIVATE_REASONING", "tool_calls": []any{map[string]any{"id": "call", "type": "function", "function": map[string]any{"name": "read", "arguments": `{"path":"PRIVATE_ARGUMENT"}`}}}}}}})
			}))
			defer provider.Close()
			policyPath := filepath.Join(t.TempDir(), "policy.csv")
			if err := os.WriteFile(policyPath, []byte("p, agent:test, *, *, allow\n"), 0600); err != nil {
				t.Fatal(err)
			}
			config := agentcore.Config{DefaultProfile: "test", MaxSteps: 2, Pools: map[string]agentcore.Pool{"mock": {BaseURL: provider.URL}}, Profiles: map[string]agentcore.Profile{"test": {Pool: "mock", Model: "test"}}}
			if failure == "model" {
				config.MaxSteps = 3
			}
			runtime, err := agentcore.New(config, "", agentFilePolicy{path: policyPath})
			if err != nil {
				t.Fatal(err)
			}
			toolCalls := 0
			tools := []agentcore.Tool{{Name: "read", Parameters: map[string]any{"type": "object"}, Execute: func(context.Context, json.RawMessage) (string, error) {
				toolCalls++
				if failure == "tool" && toolCalls == 2 {
					return "PRIVATE_TOOL_OUTPUT", errors.New("PRIVATE_TOOL_ERROR")
				}
				return "PRIVATE_TOOL_OUTPUT", nil
			}}}
			request := agentcore.RunRequest{Actor: "agent:test", RunID: "partial", Messages: []agentcore.Message{{Role: "assistant", Content: "PRIVATE_PRIOR_ASSISTANT"}, {Role: "user", Content: "PRIVATE_PRIOR_REQUEST"}}}
			dir := t.TempDir()
			cfg := agentdesk.Config{DataDir: dir, Chat: func(ctx context.Context, _ string) (string, error) {
				result, runErr := runtime.Run(ctx, request, tools)
				if runErr == nil {
					t.Fatal("expected runtime failure")
				}
				if result.Text != "" {
					t.Fatalf("regression boundary changed: Text=%q", result.Text)
				}
				return nativeFailedTurn(result, request, tools, runErr, false)
			}}
			desk, err := agentdesk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			send := httptest.NewRecorder()
			desk.Handler().ServeHTTP(send, httptest.NewRequest("POST", "http://127.0.0.1:3850/api/chat", strings.NewReader(`{"text":"Run the check"}`)))
			if send.Code != 200 {
				t.Fatal(send.Body.String())
			}
			// Reload durable state, not only the response, to prove the prose survives.
			desk, err = agentdesk.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			history := httptest.NewRecorder()
			desk.Handler().ServeHTTP(history, httptest.NewRequest("GET", "http://127.0.0.1:3850/api/messages", nil))
			var messages []agentdesk.Message
			if err = json.Unmarshal(history.Body.Bytes(), &messages); err != nil {
				t.Fatal(err)
			}
			if len(messages) != 3 || messages[1].Role != "assistant" || messages[1].Text != "First partial answer.\n\nSecond partial answer." || messages[2].Role != "error" || messages[2].Failure == nil {
				t.Fatalf("partial answer lost/duplicated: %+v", messages)
			}
			if strings.Contains(history.Body.String(), "PRIVATE_") {
				t.Fatal("non-prose private fields escaped into history")
			}
			if strings.Contains(desk.Context(), "Tool loop:") {
				t.Fatal("diagnostics entered model context")
			}
		})
	}
}

func TestNativeFailedTurnDoesNotUseHistoryOrSubscriptionDecisions(t *testing.T) {
	prior := []agentcore.Message{{Role: "assistant", Content: "PRIVATE_PRIOR"}, {Role: "user", Content: "request"}}
	request := agentcore.RunRequest{Messages: prior}
	for _, result := range []agentcore.RunResult{{}, {Messages: prior}, {Messages: append(append([]agentcore.Message{}, prior...), agentcore.Message{Role: "tool", Content: "PRIVATE_OUTPUT"})}} {
		text, err := nativeFailedTurn(result, request, nil, errors.New("PRIVATE_ERROR"), false)
		if text != "" || err == nil {
			t.Fatalf("old/tool content published: %q %v", text, err)
		}
	}
	result := agentcore.RunResult{Messages: append(append([]agentcore.Message{}, prior...), agentcore.Message{Role: "assistant", Content: `{"reply":"internal provisional reply","tool_calls":[{"arguments":{"secret":"PRIVATE_ARG"}}]}`})}
	text, err := nativeFailedTurn(result, request, nil, errors.New("PRIVATE_ERROR"), true)
	if text != "" || err == nil {
		t.Fatalf("subscription decision published: %q %v", text, err)
	}
}
