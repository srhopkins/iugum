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

func TestTurnFailureIdentifiesAttemptedCallWithoutResult(t *testing.T) {
	for _, mode := range []string{"policy", "authorization", "execution", "transport", "remote", "canceled", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			modelCalls, firstExecutions, queuedExecutions := 0, 0, 0
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				modelCalls++
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"PRIVATE_REASONING","tool_calls":[{"id":"PRIVATE_ID","type":"function","function":{"name":"mcp_editor__editor_open_beads","arguments":"{}"}},{"id":"PRIVATE_QUEUED","type":"function","function":{"name":"mcp_editor__editor_status","arguments":"{}"}}]}}]}`)
			}))
			defer model.Close()
			cause := errors.New("PRIVATE_TOKEN /private/path provider payload")
			var failure error = cause
			want := "tool_execution_error"
			switch mode {
			case "policy":
				want = "tool_policy_error"
			case "authorization":
				want = "tool_authorization_error"
			case "transport":
				want = "tool_transport_error"
				failure = NewToolFailure(want, cause)
			case "remote":
				want = "tool_remote_error"
				failure = NewToolFailure(want, cause)
			case "canceled":
				want = "canceled"
				failure = context.Canceled
				cause = context.Canceled
			case "timeout":
				want = "timeout"
				failure = context.DeadlineExceeded
				cause = context.DeadlineExceeded
			}
			tool := Tool{Name: "mcp_editor__editor_open_beads", Execute: func(context.Context, json.RawMessage) (string, error) { firstExecutions++; return "", failure }}
			var policy = gate{}
			if mode == "policy" {
				policy.deny = "agent/tool/"
			}
			if mode == "authorization" {
				tool.Authorize = func(context.Context, json.RawMessage) error { return failure }
			}
			tools := []Tool{tool, {Name: "mcp_editor__editor_status", Execute: func(context.Context, json.RawMessage) (string, error) { queuedExecutions++; return "queued", nil }}}
			runtime, err := New(config(model.URL), "", policy)
			if err != nil {
				t.Fatal(err)
			}
			request := RunRequest{Actor: "fixture", Messages: []Message{{Role: "user", Content: "PRIVATE_USER"}}}
			result, err := runtime.Run(context.Background(), request, tools)
			if err == nil || modelCalls != 1 || queuedExecutions != 0 {
				t.Fatal("hard failure replayed or recovered")
			}
			if mode != "policy" && !errors.Is(err, cause) {
				t.Fatal("error chain lost")
			}
			if (mode == "policy" || mode == "authorization") && firstExecutions != 0 {
				t.Fatal("denied tool executed")
			}
			diagnostic := runtime.EditorTurnDiagnostics(request, result, tools, true, err)
			if diagnostic.Failure == nil || diagnostic.Failure.Category != want || diagnostic.Failure.Tool != tool.Name || diagnostic.Failure.Step != 1 {
				t.Fatalf("wrong failure metadata: %+v", diagnostic.Failure)
			}
			if len(diagnostic.Calls) != 2 || diagnostic.Calls[0].Result != "no_tool_result" || diagnostic.Calls[1].Result != "no_tool_result" {
				t.Fatal("fabricated tool completion")
			}
			raw, _ := json.Marshal(diagnostic)
			for _, secret := range []string{"PRIVATE_TOKEN", "PRIVATE_REASONING", "PRIVATE_USER", "PRIVATE_ID", "PRIVATE_QUEUED", "/private/path", "payload"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("diagnostic leaked %s", secret)
				}
			}
		})
	}
}

func TestTurnFailureMetadataAllowlistAndWrapping(t *testing.T) {
	cause := errors.New("PRIVATE")
	wrapped := fmt.Errorf("PRIVATE wrapper: %w", newToolTurnError(2, "/private/tool", "tool_execution_error", NewToolFailure("PRIVATE_CATEGORY", cause)))
	diagnostic := SafeTurnFailure(wrapped)
	if diagnostic.Category != "tool_execution_error" || diagnostic.Tool != "" || diagnostic.Step != 2 || !errors.Is(wrapped, cause) {
		t.Fatal("unsafe metadata or lost cause")
	}
	if SafeTurnFailure(nil) != nil || SafeTurnFailure(cause).Category != "runtime_error" {
		t.Fatal("unknown error classification")
	}
}

func TestUnknownToolFailureOmitsModelAuthoredName(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"PRIVATE_ID","type":"function","function":{"name":"PRIVATE_UNKNOWN_TOOL","arguments":"{}"}}]}}]}`)
	}))
	defer model.Close()
	runtime, err := New(config(model.URL), "", gate{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Run(context.Background(), RunRequest{Actor: "fixture"}, nil)
	failure := SafeTurnFailure(err)
	raw, _ := json.Marshal(failure)
	if failure.Category != "tool_unknown" || failure.Tool != "" || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("unknown tool name leaked")
	}
}
