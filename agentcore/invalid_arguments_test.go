package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recoveryPolicy struct{ err error }

func (g recoveryPolicy) Enforce(_ context.Context, q contract.Request) error {
	if q.Obj == "agent/tool/read" {
		return g.err
	}
	return nil
}
func TestInvalidToolArgumentsSameTurnAndCallIDs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Messages []Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if calls == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"bad","type":"function","function":{"name":"read","arguments":"{\"name\":\"../bad\"}"}},{"id":"good","type":"function","function":{"name":"read","arguments":"{\"name\":\"valid\"}"}}]}}]}`)
			return
		}
		var found []Message
		for _, m := range body.Messages {
			if m.Role == "tool" {
				found = append(found, m)
			}
		}
		if len(found) < 2 || found[0].ToolCallID != "bad" || found[1].ToolCallID != "good" {
			t.Errorf("tool IDs: %+v", found)
		}
		if len(found) >= 2 {
			var result map[string]any
			json.Unmarshal([]byte(found[0].Content), &result)
			if result["code"] != "invalid_arguments" || result["is_error"] != true || found[1].Content != "read" {
				t.Errorf("results: %+v", found)
			}
		}
		if calls == 2 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"corrected","type":"function","function":{"name":"read","arguments":"{\"name\":\"valid\"}"}}]}}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
	}))
	defer server.Close()
	runtime, err := New(config(server.URL), "", gate{})
	if err != nil {
		t.Fatal(err)
	}
	executions := 0
	result, err := runtime.Run(context.Background(), RunRequest{Actor: "test"}, []Tool{{Name: "read", Execute: func(_ context.Context, raw json.RawMessage) (string, error) {
		executions++
		var q struct{ Name string }
		json.Unmarshal(raw, &q)
		if q.Name != "valid" {
			return "", fmt.Errorf("wrapped: %w", &InvalidToolArguments{Message: "Use a valid name"})
		}
		return "read", nil
	}}})
	if err != nil || result.Text != "done" || executions != 3 || calls != 3 {
		t.Fatalf("recovery: %+v %v calls=%d executions=%d", result, err, calls, executions)
	}
}
func TestInvalidToolArgumentsOnlyExecutionIsRecoverable(t *testing.T) {
	for _, kind := range []string{"execution", "authorization", "policy"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			executed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"read","arguments":"{}"}}]}}]}`)
			}))
			defer server.Close()
			ordinary := errors.New("execution failed")
			typed := fmt.Errorf("wrapped denial: %w", &InvalidToolArguments{Message: "fixed"})
			var policy contract.Policy = gate{}
			tool := Tool{Name: "read", Execute: func(context.Context, json.RawMessage) (string, error) { executed = true; return "", ordinary }}
			want := ordinary
			if kind == "authorization" {
				tool.Authorize = func(context.Context, json.RawMessage) error { return typed }
				want = typed
			}
			if kind == "policy" {
				policy = recoveryPolicy{typed}
				want = typed
			}
			runtime, err := New(config(server.URL), "", policy)
			if err != nil {
				t.Fatal(err)
			}
			_, err = runtime.Run(context.Background(), RunRequest{Actor: "test"}, []Tool{tool})
			if !errors.Is(err, want) || calls != 1 || (kind != "execution" && executed) {
				t.Fatalf("hard failure changed: %v calls=%d executed=%v", err, calls, executed)
			}
		})
	}
}
