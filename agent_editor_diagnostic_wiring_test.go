package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
)

// Same Runtime.Run -> diagnostic hook -> native success/failure adapter ordering
// as production, through the real durable HTTP chat handler.
func TestNativeEditorDiagnosticTurnWiring(t *testing.T) {
	for _, mode := range []string{"returned", "failed", "cancelled", "write_failure"} {
		t.Run(mode, func(t *testing.T) {
			modelCalls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelCalls++
				if modelCalls == 1 {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"PRIVATE_REASONING","tool_calls":[{"id":"PRIVATE_CALL","type":"function","function":{"name":"mcp_editor__editor_call","arguments":"{\"args\":{\"args\":{\"SECRET_KEY\":\"PRIVATE_VALUE\"}}}"}}]}}]}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
				}
			}))
			defer provider.Close()
			cfg := agentcore.Config{DefaultProfile: "fixture", MaxSteps: 2, Pools: map[string]agentcore.Pool{"fixture": {BaseURL: provider.URL}}, Profiles: map[string]agentcore.Profile{"fixture": {Pool: "fixture", Model: "synthetic"}}}
			if mode == "failed" {
				cfg.MaxSteps = 1
			}
			runtime, e := agentcore.New(cfg, "", routingTestPolicy{})
			if e != nil {
				t.Fatal(e)
			}
			root := t.TempDir()
			if mode == "write_failure" {
				target := t.TempDir()
				if e = os.Symlink(target, filepath.Join(root, "editor-diagnostics")); e != nil {
					t.Fatal(e)
				}
			}
			tools := []agentcore.Tool{{Name: "mcp_editor__editor_call", Parameters: map[string]any{"type": "object", "required": []string{"operation"}}, Execute: func(context.Context, json.RawMessage) (string, error) {
				return "", &agentcore.InvalidToolArguments{Message: "fixed safe guidance"}
			}}}
			var warning bytes.Buffer
			desk, e := agentdesk.New(agentdesk.Config{DataDir: root, Chat: func(ctx context.Context, text string) (string, error) {
				if mode == "cancelled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				request := agentcore.RunRequest{Actor: "fixture", Profile: "fixture", RunID: "this-turn", Messages: []agentcore.Message{{Role: "user", Content: text}}}
				result, runErr := runtime.Run(ctx, request, tools)
				beforeResult, _ := json.Marshal(result)
				beforeRequest, _ := json.Marshal(request)
				recordNativeEditorTurn(runtime, request, result, tools, root, runErr, &warning)
				afterResult, _ := json.Marshal(result)
				afterRequest, _ := json.Marshal(request)
				if !bytes.Equal(beforeResult, afterResult) || !bytes.Equal(beforeRequest, afterRequest) {
					t.Error("diagnostic hook mutated runtime result or model request")
				}
				if runErr != nil {
					return nativeFailedTurn(result, request, tools, runErr, false)
				}
				return result.Text, nil
			}})
			if e != nil {
				t.Fatal(e)
			}
			response := httptest.NewRecorder()
			desk.Handler().ServeHTTP(response, httptest.NewRequest("POST", "http://127.0.0.1:3850/api/chat", strings.NewReader(`{"text":"PRIVATE_USER"}`)))
			if response.Code != 200 {
				t.Fatalf("HTTP chat outcome changed: %d", response.Code)
			}
			data, readErr := os.ReadFile(filepath.Join(root, "editor-diagnostics", "latest.json"))
			if mode == "write_failure" {
				if readErr == nil || !strings.Contains(warning.String(), "snapshot unavailable") || !strings.Contains(response.Body.String(), "done") {
					t.Fatal("diagnostic write failure changed successful turn")
				}
				return
			}
			if readErr != nil {
				t.Fatal("actual chat turn did not persist diagnostic", readErr)
			}
			var diagnostic agentcore.EditorTurnDiagnostic
			if json.Unmarshal(data, &diagnostic) != nil {
				t.Fatal("invalid persisted diagnostic")
			}
			expected := "returned"
			if mode == "failed" || mode == "cancelled" {
				expected = "error"
			}
			if diagnostic.RuntimeOutcome != expected {
				t.Fatal("wrong runtime outcome")
			}
			if mode != "cancelled" && (len(diagnostic.Calls) != 1 || diagnostic.Calls[0].Result != "recoverable_invalid_arguments") {
				t.Fatal("actual runtime validation marker lost")
			}
			for _, secret := range []string{"PRIVATE_USER", "PRIVATE_REASONING", "PRIVATE_VALUE", "SECRET_KEY", "PRIVATE_CALL"} {
				if bytes.Contains(data, []byte(secret)) {
					t.Fatal("private data reached persisted diagnostic")
				}
			}
			expectedCalls := 2
			if mode == "failed" {
				expectedCalls = 1
			}
			if mode == "cancelled" {
				expectedCalls = 0
			}
			if modelCalls != expectedCalls {
				t.Fatal("diagnostic hook changed model call count")
			}
		})
	}
}
