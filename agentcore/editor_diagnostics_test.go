package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func diagnosticTestTools() []Tool {
	return []Tool{{Name: "mcp_editor__editor_call", Description: "PRIVATE_DESCRIPTION", Parameters: map[string]any{"type": "object", "additionalProperties": false, "required": []any{"instance", "commandId", "operation", "args"}, "properties": map[string]any{"instance": map[string]any{"type": "string", "format": "uuid"}, "commandId": map[string]any{"type": "string", "format": "uuid"}, "operation": map[string]any{"type": "string"}, "args": map[string]any{"type": "object"}}}}}
}
func TestEditorDiagnosticsOnlyKnownShapesAndHostValidationMarker(t *testing.T) {
	r, _ := New(Config{DefaultProfile: "fixture", Pools: map[string]Pool{"fixture": {BaseURL: "http://localhost:1"}}, Profiles: map[string]Profile{"fixture": {Pool: "fixture", Model: "gpt-4o-mini"}}, Instructions: "PRIVATE_ROLE"}, "", gate{})
	r.records = []UsageRecord{{RunID: "this", Profile: "fixture", Model: "gpt-4o-mini"}, {RunID: "other", Profile: "PRIVATE_OTHER", Model: "PRIVATE_OTHER"}}
	q := RunRequest{RunID: "this", Profile: "fixture", Messages: []Message{{Role: "user", Content: "PRIVATE_USER"}}}
	messages := append([]Message{}, q.Messages...)
	messages = append(messages, Message{Role: "assistant", Content: "PRIVATE_REASONING", ToolCalls: []ToolCall{{ID: "PRIVATE_CALL_ID", Function: FunctionCall{Name: "mcp_editor__editor_call", Arguments: `{"instance":"PRIVATE_UUID","commandId":"11111111-1111-4111-8111-111111111111","args":{"args":{"operation":"PRIVATE_OPERATION","SECRET_KEY":"PRIVATE_SECRET"},"path":"PRIVATE_FILE"},"SECRET_TOP":"PRIVATE_TOKEN"}`}}, {ID: "other", Function: FunctionCall{Name: "shell", Arguments: `{"command":"PRIVATE_SHELL"}`}}}}, Message{Role: "tool", ToolCallID: "PRIVATE_CALL_ID", Content: `{"is_error":true,"code":"invalid_arguments","message":"PRIVATE_OUTPUT"}`, ToolValidationCode: "invalid_arguments"})
	d := r.EditorTurnDiagnostics(q, RunResult{Messages: messages}, diagnosticTestTools(), true)
	raw, _ := json.Marshal(d)
	for _, secret := range []string{"PRIVATE_", "SECRET_KEY", "SECRET_TOP", "shell", "PRIVATE_CALL_ID"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("leaked diagnostic content %q", secret)
		}
	}
	if len(d.Calls) != 1 || d.Calls[0].Result != "recoverable_invalid_arguments" || d.Calls[0].UnknownFields != 1 || d.RuntimeOutcome != "error" || len(d.Models) != 1 {
		t.Fatalf("metadata: %+v", d)
	}
	if d.Calls[0].Fields[2].Present || d.Calls[0].Fields[0].UUIDValid == nil || *d.Calls[0].Fields[0].UUIDValid {
		t.Fatal("missing operation or invalid UUID shape lost")
	}
	found := false
	for _, issue := range d.Calls[0].Issues {
		found = found || issue.Code == "required" && issue.Field == "operation"
	}
	if !found {
		t.Fatal("missing required field diagnostic")
	}
	// Untrusted tool output cannot invent a host-local validation result.
	messages[len(messages)-1].ToolValidationCode = ""
	d = r.EditorTurnDiagnostics(q, RunResult{Messages: messages}, diagnosticTestTools(), false)
	if d.Calls[0].Result != "tool_result" || d.RuntimeOutcome != "returned" {
		t.Fatal("tool output changed host classification")
	}
}
func TestEditorDiagnosticsCapsAndHistoryExclusion(t *testing.T) {
	r, _ := New(config("http://localhost:1"), "", gate{})
	q := RunRequest{Messages: []Message{{Role: "assistant", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "mcp_editor__editor_call", Arguments: `{}`}}}}}}
	messages := append([]Message{}, q.Messages...)
	for i := 0; i < 100; i++ {
		messages = append(messages, Message{Role: "assistant", ToolCalls: []ToolCall{{ID: fmt.Sprint(i), Function: FunctionCall{Name: "mcp_editor__editor_call", Arguments: `{"args":{"args":{"args":{"SECRET":"VALUE"}}}}`}}}})
	}
	d := r.EditorTurnDiagnostics(q, RunResult{Messages: messages}, diagnosticTestTools(), true)
	encoded, _ := json.Marshal(d)
	if !d.Truncated || len(d.Calls) > MaxEditorDiagnosticRecords || len(encoded) > MaxEditorDiagnosticBytes || strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "VALUE") {
		t.Fatal("diagnostic cap/privacy failure")
	}
	empty := r.EditorTurnDiagnostics(q, RunResult{}, diagnosticTestTools(), true)
	if len(empty.Calls) != 0 {
		t.Fatal("request history mistaken for current calls")
	}
}
func TestEditorValidationMarkerIsNotModelVisible(t *testing.T) {
	m := Message{Role: "tool", Content: "safe", ToolValidationCode: "PRIVATE_VALIDATION"}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "PRIVATE_VALIDATION") || strings.Contains(string(raw), "ToolValidationCode") {
		t.Fatal("host validation metadata injected into model wire")
	}
	wire, e := modelMessages([]Message{m}, false)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(wire)
	if strings.Contains(string(raw), "PRIVATE_VALIDATION") {
		t.Fatal("metadata entered actual model conversion")
	}

}
func TestEditorDiagnosticMarkerAcrossRuntimeOutcomes(t *testing.T) {
	for _, mode := range []string{"recovered", "hard", "denied", "cap"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				if calls > 1 && mode != "cap" {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
					return
				}
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"mcp_editor__editor_call","arguments":"{}"}}]}}]}`)
			}))
			defer server.Close()
			cfg := config(server.URL)
			cfg.MaxSteps = 2
			r, _ := New(cfg, "", gate{})
			tool := diagnosticTestTools()[0]
			tool.Execute = func(context.Context, json.RawMessage) (string, error) {
				if mode == "hard" {
					return "", errors.New("PRIVATE_HARD_ERROR")
				}
				return "", &InvalidToolArguments{Message: "fixed guidance"}
			}
			if mode == "denied" {
				tool.Authorize = func(context.Context, json.RawMessage) error { return errors.New("PRIVATE_DENIAL") }
			}
			q := RunRequest{Actor: "fixture", RunID: "fixture"}
			result, err := r.Run(context.Background(), q, []Tool{tool})
			d := r.EditorTurnDiagnostics(q, result, []Tool{tool}, err != nil)
			if len(d.Calls) == 0 {
				t.Fatal("lost failed or successful editor evidence")
			}
			want := "recoverable_invalid_arguments"
			if mode == "hard" || mode == "denied" {
				want = "no_tool_result"
			}
			if d.Calls[0].Result != want {
				t.Fatalf("mode=%s result=%s", mode, d.Calls[0].Result)
			}
			if (mode == "recovered") != (err == nil) {
				t.Fatal("runtime outcome changed")
			}
		})
	}
}

func TestEditorDiagnosticsMalformedAndCancelled(t *testing.T) {
	r, _ := New(config("http://localhost:1"), "", gate{})
	q := RunRequest{Actor: "fixture"}
	result := RunResult{Messages: []Message{{Role: "assistant", ToolCalls: []ToolCall{{Function: FunctionCall{Name: "mcp_editor__editor_call", Arguments: `{PRIVATE_PARSE_SECRET`}}}}}}
	d := r.EditorTurnDiagnostics(q, result, diagnosticTestTools(), true)
	raw, _ := json.Marshal(d)
	if len(d.Calls) != 1 || d.Calls[0].Shape != "not_object" || strings.Contains(string(raw), "PRIVATE_PARSE_SECRET") {
		t.Fatal("malformed input leaked or lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := r.Run(ctx, q, nil)
	if err == nil {
		t.Fatal("cancelled run succeeded")
	}
	d = r.EditorTurnDiagnostics(q, result, nil, true)
	if len(d.Calls) != 0 || d.RuntimeOutcome != "error" {
		t.Fatal("cancelled run diagnostics changed")
	}
}

func TestEditorDiagnosticsFlatBeadsKnownFieldsOnly(t *testing.T) {
	r, _ := New(config("http://localhost:1"), "", gate{})
	tool := Tool{Name: "mcp_editor__editor_open_beads", Parameters: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"instance", "commandId"}, "properties": map[string]any{"instance": map[string]any{"type": "string", "format": "uuid"}, "commandId": map[string]any{"type": "string", "format": "uuid"}, "view": map[string]any{"type": "string"}}}}
	result := RunResult{Messages: []Message{{Role: "assistant", ToolCalls: []ToolCall{{ID: "PRIVATE_ID", Function: FunctionCall{Name: tool.Name, Arguments: `{"instance":"11111111-1111-4111-8111-111111111111","commandId":"22222222-2222-4222-8222-222222222222","view":"PRIVATE_VIEW","args":{"operation":"PRIVATE_OPERATION"},"PRIVATE_KEY":"PRIVATE_VALUE"}`}}}}}}
	d := r.EditorTurnDiagnostics(RunRequest{}, result, []Tool{tool}, true)
	raw, _ := json.Marshal(d)
	if len(d.Calls) != 1 || strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("flat diagnostics missing or private data leaked")
	}
	found := false
	for _, field := range d.Calls[0].Fields {
		found = found || field.Field == "view" && field.Present && field.Type == "string"
	}
	if !found {
		t.Fatal("flat known field shape omitted")
	}
}
