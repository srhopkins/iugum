package agentmcp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testGate struct {
	denied     atomic.Bool
	denyAction string
}

func (g *testGate) Enforce(ctx context.Context, r contract.Request) error {
	if g.denied.Load() || r.Act == g.denyAction {
		return errors.New("denied")
	}
	return nil
}
func TestHTTPToolsPolicyRechecked(t *testing.T) {
	var calls atomic.Int64
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "Echo synthetic text"}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) {
		calls.Add(1)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Text}}}, nil, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	defer httpServer.Close()
	g := &testGate{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools, close, e := Connect(ctx, []Config{{Name: "demo", URL: httpServer.URL}}, "agent:test", g)
	if e != nil {
		t.Fatal(e)
	}
	defer close()
	if len(tools) != 1 || tools[0].Name != "mcp_demo__echo" {
		t.Fatal(tools)
	}
	v, e := tools[0].Execute(ctx, json.RawMessage(`{"text":"synthetic passage"}`))
	if e != nil || !strings.Contains(v, "synthetic passage") {
		t.Fatal(v, e)
	}
	g.denied.Store(true)
	if _, e = tools[0].Execute(ctx, json.RawMessage(`{"text":"forbidden"}`)); e == nil || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
	close()
	close()
}
func TestDeniedConnectionDoesNotContactServer(t *testing.T) {
	var calls atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer s.Close()
	g := &testGate{denyAction: "connect"}
	_, _, e := Connect(context.Background(), []Config{{Name: "denied", URL: s.URL}}, "agent:test", g)
	if e == nil || calls.Load() != 0 {
		t.Fatal(e, calls.Load())
	}
}
func TestUnavailableConfiguredServerFailsClearly(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer s.Close()
	_, _, e := Connect(context.Background(), []Config{{Name: "missing", URL: s.URL}}, "agent:test", &testGate{})
	if e == nil || !strings.Contains(e.Error(), "MCP missing unavailable") {
		t.Fatal(e)
	}
}
