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

func TestArgumentsRecoverBeforeRemoteExecution(t *testing.T) {
	var executions atomic.Int64
	server := mcp.NewServer(&mcp.Implementation{Name: "arguments", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "editor"}, func(_ context.Context, _ *mcp.CallToolRequest, args struct {
		Instance  string         `json:"instance"`
		Operation string         `json:"operation"`
		CommandID string         `json:"commandId"`
		Args      map[string]any `json:"args"`
	}) (*mcp.CallToolResult, any, error) {
		executions.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "opened"}}}, nil, nil
	})
	sdk := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer sdk.Close()
	gate := &testGate{}
	tools, closeTools, err := Connect(context.Background(), []Config{{Name: "demo", URL: sdk.URL}}, "test", gate)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTools()
	for _, raw := range []string{`{"secret":"never-echo-this"}`, `{"instance":5,"operation":"open"}`, `null`, `{`} {
		_, err := tools[0].Execute(context.Background(), json.RawMessage(raw))
		var invalid *agentcore.InvalidToolArguments
		if !errors.As(err, &invalid) || strings.Contains(err.Error(), "never-echo-this") || executions.Load() != 0 {
			t.Fatalf("unsafe validation: %v executions=%d", err, executions.Load())
		}
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Messages []agentcore.Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch calls {
		case 1:
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"bad","type":"function","function":{"name":"mcp_demo__editor","arguments":"{}"}}]}}]}`)
		case 2:
			if executions.Load() != 0 {
				t.Error("invalid attempt reached remote handler")
			}
			last := body.Messages[len(body.Messages)-1]
			if last.ToolCallID != "bad" || !strings.Contains(last.Content, "invalid_arguments") || !strings.Contains(last.Content, "missing required field arguments.instance") {
				t.Errorf("missing correction feedback: %+v", last)
			}
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"good","type":"function","function":{"name":"mcp_demo__editor","arguments":"{\"instance\":\"exact\",\"operation\":\"open_beads\",\"commandId\":\"new-id\",\"args\":{}}"}}]}}]}`)
		default:
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
		}
	}))
	defer model.Close()
	cfg := agentcore.Config{DefaultProfile: "test", Pools: map[string]agentcore.Pool{"test": {Provider: "openai", BaseURL: model.URL}}, Profiles: map[string]agentcore.Profile{"test": {Pool: "test", Model: "synthetic"}}}
	runtime, err := agentcore.New(cfg, "", gate)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Run(context.Background(), agentcore.RunRequest{Actor: "test"}, tools)
	if err != nil || result.Text != "done" || calls != 3 || executions.Load() != 1 {
		t.Fatalf("same-turn recovery: %+v %v calls=%d executions=%d", result, err, calls, executions.Load())
	}
	gate.denied.Store(true)
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	var invalid *agentcore.InvalidToolArguments
	if err == nil || errors.As(err, &invalid) || executions.Load() != 1 {
		t.Fatalf("denial became recoverable: %v", err)
	}
	gate.denied.Store(false)
	sdk.Close()
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{"instance":"exact","operation":"open_beads","commandId":"new-id","args":{}}`))
	if err == nil || errors.As(err, &invalid) || executions.Load() != 1 {
		t.Fatalf("transport became recoverable: %v", err)
	}
}

func TestRemoteErrorIsNotArgumentRecovery(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "error", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "error"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "permission denied: missing required arguments"}}}, nil, nil
	})
	sdk := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer sdk.Close()
	tools, closeTools, err := Connect(context.Background(), []Config{{Name: "demo", URL: sdk.URL}}, "test", &testGate{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTools()
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	var invalid *agentcore.InvalidToolArguments
	if err == nil || errors.As(err, &invalid) {
		t.Fatalf("remote failure reclassified: %v", err)
	}
}
