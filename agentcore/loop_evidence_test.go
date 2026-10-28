package agentcore

import (
	"strings"
	"testing"
)

func TestFailedToolLoopSummaryOmitsPrivateData(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "PRIVATE_TRANSCRIPT"},
		{Role: "assistant", Content: "PRIVATE_REASONING", ToolCalls: []ToolCall{{ID: "PRIVATE_ID", Function: FunctionCall{Name: "clone_read", Arguments: "PRIVATE_ARGS"}}, {ID: "pending", Function: FunctionCall{Name: "clone_write"}}}},
		{Role: "tool", ToolCallID: "PRIVATE_ID", Content: `{"is_error":true,"code":"invalid_arguments","message":"PRIVATE_OUTPUT"}`},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "secret", Function: FunctionCall{Name: "unknown_PRIVATE_NAME"}}, {ID: "ok", Function: FunctionCall{Name: "shell"}}, {ID: "unsafe", Function: FunctionCall{Name: "/private/path"}}}},
		{Role: "tool", ToolCallID: "ok", Content: "PRIVATE_SHELL_OUTPUT"},
		{Role: "tool", ToolCallID: "secret", Content: "PRIVATE_UNKNOWN_OUTPUT"},
		{Role: "tool", ToolCallID: "unsafe", Content: "PRIVATE_UNSAFE_OUTPUT"},
	}
	summary := FailedToolLoopSummary(messages, []Tool{{Name: "clone_read"}, {Name: "clone_write"}, {Name: "shell"}, {Name: "/private/path"}})
	want := "Tool loop: result=failed steps=2; step=1 clone_read=recoverable_invalid_arguments; step=2 shell=completed"
	if summary != want {
		t.Fatalf("summary: %s", summary)
	}
	if strings.Contains(summary, "PRIVATE") || strings.Contains(summary, "clone_write") {
		t.Fatal(summary)
	}
}
func TestFailedToolLoopSummaryBounded(t *testing.T) {
	var messages []Message
	for i := 0; i < 100; i++ {
		messages = append(messages, Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "same", Function: FunctionCall{Name: "read"}}}}, Message{Role: "tool", ToolCallID: "same", Content: "private"})
	}
	result := FailedToolLoopSummary(messages, []Tool{{Name: "read"}})
	if strings.Count(result, "read=completed") != 16 || !strings.Contains(result, "steps=100") || !strings.HasSuffix(result, "additional_completed_calls_omitted") {
		t.Fatal(result)
	}
}

func TestFailedToolLoopSummaryErrorCategories(t *testing.T) {
	for _, tc := range []struct{ name, body, category string }{
		{"unknown skill", `{"is_error":true,"code":"unknown_skill","message":"PRIVATE_MESSAGE","available_skills":["PRIVATE_SKILL"],"default_path":"PRIVATE_PATH"}`, "recoverable_unknown_skill"},
		{"invalid path", `{"is_error":true,"code":"invalid_path","message":"PRIVATE_PATH"}`, "recoverable_invalid_path"},
		{"generic error", `{"is_error":true}`, "error"},
		{"unknown private code", `{"is_error":true,"code":"PRIVATE_CODE"}`, "error"},
		{"non error code", `{"is_error":false,"code":"unknown_skill"}`, "completed"},
		{"plain output", "PRIVATE_OUTPUT", "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []Message{
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "PRIVATE_ID", Function: FunctionCall{Name: "skill_read", Arguments: "PRIVATE_ARGS"}}}},
				{Role: "tool", ToolCallID: "PRIVATE_ID", Content: tc.body},
			}
			got := FailedToolLoopSummary(messages, []Tool{{Name: "skill_read"}})
			want := "Tool loop: result=failed steps=1; step=1 skill_read=" + tc.category
			if got != want {
				t.Fatalf("got %q; want %q", got, want)
			}
			if strings.Contains(got, "success") || strings.Contains(got, "PRIVATE") {
				t.Fatal(got)
			}
		})
	}
}
