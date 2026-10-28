package agentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentcore"
)

func TestEditorFailureExactAllowlist(t *testing.T) {
	cases := map[string]string{
		"editor instance disconnected":      "editor_instance_disconnected",
		"editor commandId already reserved": "editor_command_id_reserved",
		"editor capability unavailable":     "editor_capability_unavailable",
		"editor queue full":                 "editor_queue_full",
		"editor command not found for this instance and commandId; use the exact ID from a queued receipt, not a rejected or invented ID; a missing receipt does not mean an editor capability is unsupported": "editor_receipt_not_found",
	}
	for text, want := range cases {
		result := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
		if got := editorFailureCategory("editor", "editor_open_beads", result); got != want {
			t.Fatalf("fixed rejection %s", got)
		}
		for _, name := range []string{"PRIVATE_PREFIX " + text, text + " PRIVATE_SUFFIX", "unknown PRIVATE_PAYLOAD"} {
			result.Content = []mcp.Content{&mcp.TextContent{Text: name}}
			if editorFailureCategory("editor", "editor_open_beads", result) != "" {
				t.Fatal("inexact rejection classified")
			}
		}
		result.Content = []mcp.Content{&mcp.TextContent{Text: text}}
		if editorFailureCategory("other", "editor_open_beads", result) != "" || editorFailureCategory("editor", "other", result) != "" {
			t.Fatal("unrelated server/tool classified")
		}
		result.IsError = false
		if editorFailureCategory("editor", "editor_open_beads", result) != "" {
			t.Fatal("success classified")
		}
		result.IsError = true
		result.Content = append(result.Content, &mcp.TextContent{Text: "PRIVATE"})
		if editorFailureCategory("editor", "editor_open_beads", result) != "" {
			t.Fatal("additional private content classified")
		}
	}
}

func TestEditorRemoteFailureRemainsHardStop(t *testing.T) {
	for _, mode := range []string{"known", "unknown", "denied", "transport"} {
		t.Run(mode, func(t *testing.T) {
			var executions atomic.Int64
			server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
			mcp.AddTool(server, &mcp.Tool{Name: "editor_open_beads"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
				executions.Add(1)
				text := "editor instance disconnected"
				if mode == "unknown" {
					text = "PRIVATE_PREFIX editor instance disconnected PRIVATE_TOKEN"
				}
				return nil, nil, errors.New(text)
			})
			sdk := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
			defer sdk.Close()
			policy := &testGate{}
			tools, closeTools, err := Connect(context.Background(), []Config{{Name: "editor", URL: sdk.URL}}, "fixture", policy)
			if err != nil {
				t.Fatal(err)
			}
			defer closeTools()
			if mode == "denied" {
				policy.denied.Store(true)
			}
			if mode == "transport" {
				sdk.Close()
			}
			modelCalls := 0
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				modelCalls++
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"mcp_editor__editor_open_beads","arguments":"{}"}},{"id":"two","type":"function","function":{"name":"mcp_editor__editor_open_beads","arguments":"{}"}}]}}]}`)
			}))
			defer model.Close()
			cfg := agentcore.Config{DefaultProfile: "fixture", Pools: map[string]agentcore.Pool{"fixture": {BaseURL: model.URL}}, Profiles: map[string]agentcore.Profile{"fixture": {Pool: "fixture", Model: "fixture"}}}
			runtime, err := agentcore.New(cfg, "", &testGate{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := runtime.Run(context.Background(), agentcore.RunRequest{Actor: "fixture"}, tools)
			var invalid *agentcore.InvalidToolArguments
			if err == nil || errors.As(err, &invalid) || modelCalls != 1 {
				t.Fatal("hard failure recovered or replayed")
			}
			want := "editor_instance_disconnected"
			expected := int64(1)
			switch mode {
			case "unknown":
				want = "tool_remote_error"
			case "denied":
				want = "tool_authorization_error"
				expected = 0
			case "transport":
				want = "tool_transport_error"
				expected = 0
			}
			failure := agentcore.SafeTurnFailure(err)
			if failure.Category != want || failure.Tool != "mcp_editor__editor_open_beads" || failure.Step != 1 || executions.Load() != expected {
				t.Fatalf("wrong hard failure evidence %+v executions=%d", failure, executions.Load())
			}
			raw, _ := json.Marshal(failure)
			if strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("private cause leaked")
			}
			if len(result.Messages) != 1 {
				t.Fatal("unexpected fabricated result")
			}
		})
	}
}
