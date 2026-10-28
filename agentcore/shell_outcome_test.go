package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellOutcomeInternalAndRecoverable(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "ExitCode") || strings.Contains(string(body), "Outcome") || strings.Contains(string(body), "PRIVATE_CATEGORY") {
			t.Error("internal metadata reached provider")
		}
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"shell","arguments":"{}"}}]}}]}`)
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"recovered"}}]}`)
		}
	}))
	defer server.Close()
	runtime, err := New(config(server.URL), filepath.Join(t.TempDir(), "usage.json"), gate{})
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool{Name: "shell", Parameters: map[string]any{"type": "object"}, ExecuteResult: func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Text: "PRIVATE_OUTPUT", Outcome: &ToolOutcome{Category: "exit", ExitCode: 7}}, nil
	}}
	result, err := runtime.Run(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "run"}}}, []Tool{tool})
	if err != nil || result.Text != "recovered" || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	summary := FailedToolLoopSummary(result.Messages, []Tool{tool})
	if !strings.Contains(summary, "shell=exit_7") || strings.Contains(summary, "PRIVATE_OUTPUT") {
		t.Fatal(summary)
	}
	data, _ := json.Marshal(result.Messages)
	if strings.Contains(string(data), "Outcome") || strings.Contains(string(data), "ExitCode") {
		t.Fatal("metadata persisted")
	}
	data, _ = json.Marshal(ToolResult{Outcome: &ToolOutcome{Category: "PRIVATE_CATEGORY"}})
	if strings.Contains(string(data), "PRIVATE_CATEGORY") {
		t.Fatal("tool metadata serialized")
	}
}
func TestShellOutcomeErrorsRetained(t *testing.T) {
	for _, category := range []string{"timeout", "canceled", "launch_error"} {
		t.Run(category, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"shell","arguments":"{}"}}]}}]}`)
			}))
			defer server.Close()
			runtime, _ := New(config(server.URL), filepath.Join(t.TempDir(), "usage.json"), gate{})
			tool := Tool{Name: "shell", Parameters: map[string]any{"type": "object"}, ExecuteResult: func(context.Context, json.RawMessage) (ToolResult, error) {
				return ToolResult{Text: "PRIVATE_OUTPUT", Outcome: &ToolOutcome{Category: category}}, fmt.Errorf("failure")
			}}
			result, err := runtime.Run(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "run"}}}, []Tool{tool})
			summary := FailedToolLoopSummary(result.Messages, []Tool{tool})
			if err == nil || !strings.Contains(summary, "shell="+category) || strings.Contains(summary, "PRIVATE_OUTPUT") {
				t.Fatalf("summary=%s err=%v", summary, err)
			}
		})
	}
}
func TestShellOutcomeDoesNotTrustOutputOrFreeformMetadata(t *testing.T) {
	tool := Tool{Name: "shell"}
	messages := []Message{{Role: "assistant", ToolCalls: []ToolCall{{ID: "x", Function: FunctionCall{Name: "shell"}}}}, {Role: "tool", ToolCallID: "x", Content: "[exit status 7]", ToolOutcome: &ToolOutcome{Category: "PRIVATE_CATEGORY"}}}
	summary := FailedToolLoopSummary(messages, []Tool{tool})
	if !strings.Contains(summary, "shell=completed") || strings.Contains(summary, "PRIVATE") {
		t.Fatal(summary)
	}
}
