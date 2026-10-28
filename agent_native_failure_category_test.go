package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// Exercise the actual projection, private snapshot, and HTTP feedback path.
// A reused tool name on step 2 must identify the abort, never its queued sibling.
func TestNativeFailureCategoryPersistenceAndUI(t *testing.T) {
	for _, failStep := range []int{1, 2} {
		t.Run(fmt.Sprint(failStep), func(t *testing.T) {
			modelCalls, attempts, queued := 0, 0, 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				modelCalls++
				if modelCalls < failStep {
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"first","type":"function","function":{"name":"mcp_editor__editor_open_beads","arguments":"{}"}}]}}]}`)
					return
				}
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"PRIVATE_REASONING","tool_calls":[{"id":"failure","type":"function","function":{"name":"mcp_editor__editor_open_beads","arguments":"{}"}},{"id":"queued","type":"function","function":{"name":"mcp_editor__editor_status","arguments":"{}"}}]}}]}`)
			}))
			defer provider.Close()
			cfg := agentcore.Config{DefaultProfile: "fixture", MaxSteps: 3, Pools: map[string]agentcore.Pool{"fixture": {BaseURL: provider.URL}}, Profiles: map[string]agentcore.Profile{"fixture": {Pool: "fixture", Model: "fixture"}}}
			runtime, err := agentcore.New(cfg, "", routingTestPolicy{})
			if err != nil {
				t.Fatal(err)
			}
			cause := errors.New("PRIVATE_TOKEN /private/provider payload")
			tools := []agentcore.Tool{
				{Name: "mcp_editor__editor_open_beads", Execute: func(context.Context, json.RawMessage) (string, error) {
					attempts++
					if attempts < failStep {
						return "completed", nil
					}
					return "", agentcore.NewToolFailure("editor_instance_disconnected", cause)
				}},
				{Name: "mcp_editor__editor_status", Execute: func(context.Context, json.RawMessage) (string, error) { queued++; return "unexpected", nil }},
			}
			root := t.TempDir()
			desk, err := agentdesk.New(agentdesk.Config{DataDir: root, Chat: func(ctx context.Context, text string) (string, error) {
				request := agentcore.RunRequest{Actor: "fixture", Messages: []agentcore.Message{{Role: "user", Content: text}}}
				result, runErr := runtime.Run(ctx, request, tools)
				if !errors.Is(runErr, cause) {
					t.Error("wrapped cause lost")
				}
				recordNativeEditorTurn(runtime, request, result, tools, root, runErr, nil)
				return nativeFailedTurn(result, request, tools, runErr, false)
			}})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			desk.Handler().ServeHTTP(response, httptest.NewRequest("POST", "http://127.0.0.1:3850/api/chat", strings.NewReader(`{"text":"PRIVATE_USER"}`)))
			data, err := os.ReadFile(filepath.Join(root, "editor-diagnostics", "latest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var diagnostic agentcore.EditorTurnDiagnostic
			if json.Unmarshal(data, &diagnostic) != nil || diagnostic.Failure == nil {
				t.Fatal("missing persisted failure")
			}
			if diagnostic.Failure.Category != "editor_instance_disconnected" || diagnostic.Failure.Tool != "mcp_editor__editor_open_beads" || diagnostic.Failure.Step != failStep {
				t.Fatalf("wrong failure %+v", diagnostic.Failure)
			}
			body := response.Body.String()
			if response.Code != 200 || !strings.Contains(body, "Failure category: editor_instance_disconnected; tool=mcp_editor__editor_open_beads; step="+fmt.Sprint(failStep)) {
				t.Fatalf("missing safe UI feedback: %s", body)
			}
			for _, private := range []string{"PRIVATE_TOKEN", "PRIVATE_REASONING", "PRIVATE_USER", "/private/provider", "payload"} {
				if bytes.Contains(data, []byte(private)) || strings.Contains(body, private) {
					t.Fatal("private data leaked")
				}
			}
			if modelCalls != failStep || attempts != failStep || queued != 0 {
				t.Fatal("failure retried or queued action replayed")
			}
			if diagnostic.Calls[len(diagnostic.Calls)-1].Result != "no_tool_result" {
				t.Fatal("queued action reported completed")
			}
		})
	}
}
