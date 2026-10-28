package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/agentplatform/browsergrant"
	"net/http"
)

func (a *BrowserGrantAdapter) initMCP() {
	server := mcp.NewServer(&mcp.Implementation{Name: "iugum-browser", Version: "2"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "browser_windows", Description: "List confirmed exact browser pairs and verified observation phase. Pairing alone does not prove driver readiness."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		v, e := a.browserWindows(ctx)
		return nil, v, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "browser_call", Description: "Queue one typed observe, click or scroll at an exact confirmed pair with a fresh commandId. Never retry uncertain actions; read browser_status."}, func(ctx context.Context, _ *mcp.CallToolRequest, q struct {
		Pair        string         `json:"pair"`
		CommandID   string         `json:"commandId"`
		Operation   string         `json:"operation"`
		Observation string         `json:"observation"`
		Args        map[string]any `json:"args"`
	}) (*mcp.CallToolResult, any, error) {
		s := a.server
		s.host.mu.Lock()
		defer s.host.mu.Unlock()
		if e := s.hostCheck(ctx, "browser_transport", "write"); e != nil {
			return nil, nil, e
		}
		host, e := a.browserHost(ctx, q.Pair)
		if e != nil {
			return nil, nil, e
		}
		v, e := a.dispatcher.QueueForWindow(ctx, host.Instance, windowGeneration(host), browsergrant.Request{Pair: q.Pair, CommandID: q.CommandID, Operation: q.Operation, Observation: q.Observation, Args: q.Args})
		return nil, v, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "browser_status", Description: "Read the original typed browser receipt and verified PNG observation, if retained. Historical image does not authorize input."}, func(ctx context.Context, _ *mcp.CallToolRequest, q struct {
		Pair      string `json:"pair"`
		CommandID string `json:"commandId"`
	}) (*mcp.CallToolResult, any, error) {
		s := a.server
		s.host.mu.Lock()
		defer s.host.mu.Unlock()
		if e := s.hostCheck(ctx, "browser_transport", "read"); e != nil {
			return nil, nil, e
		}
		// A terminal uncertain result may detach its pair. Resolve original window
		// from the durable command, but never infer another window or expose leases.
		var host HostIdentity
		var command browsergrant.DispatchCommand
		var err error
		for _, w := range s.host.windows {
			candidate := HostIdentity{Instance: w.Instance, Lease: w.Lease}
			if !s.pairingWindow(candidate) {
				continue
			}
			command, err = a.dispatcher.StatusForWindow(ctx, q.Pair, w.Instance, windowGeneration(candidate), q.CommandID)
			if err == nil {
				host = candidate
				break
			}
		}
		if host.Instance == "" {
			return nil, nil, errors.New("exact current browser receipt window unavailable")
		}
		image, e := a.dispatcher.ImageForWindow(ctx, q.Pair, host.Instance, windowGeneration(host), q.CommandID)
		if e != nil {
			return nil, nil, e
		}
		metadata := map[string]any{"command": command}
		contents := []mcp.Content{}
		if image != nil {
			metadata["fresh"] = image.Fresh
			metadata["observation"] = image.Record
			metadata["viewport"] = image.Viewport
			metadata["targets"] = image.Targets
			contents = append(contents, &mcp.ImageContent{MIMEType: "image/png", Data: image.Raster})
		}
		b, e := json.Marshal(metadata)
		if e != nil {
			return nil, nil, e
		}
		if len(b) > 64*1024 {
			return nil, nil, errors.New("browser metadata exceeds bound")
		}
		contents = append(contents, &mcp.TextContent{Text: string(b)})
		return &mcp.CallToolResult{Content: contents}, nil, nil
	})
	a.handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

// Caller holds host.mu; all registry to dispatcher lock ordering is consistent.
func (a *BrowserGrantAdapter) browserHost(ctx context.Context, pair string) (HostIdentity, error) {
	windows, e := a.dispatcher.Windows(ctx)
	if e != nil {
		return HostIdentity{}, e
	}
	for _, v := range windows {
		if v.Pair != pair {
			continue
		}
		w, ok := a.server.host.windows[v.Instance]
		if !ok {
			break
		}
		host := HostIdentity{Instance: v.Instance, Lease: w.Lease}
		if a.server.pairingWindow(host) && windowGeneration(host) == v.Window {
			return host, nil
		}
	}
	return HostIdentity{}, errors.New("confirmed pair/current editor window required")
}
func (a *BrowserGrantAdapter) browserWindows(ctx context.Context) ([]browsergrant.DispatchWindow, error) {
	s := a.server
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	if e := s.hostCheck(ctx, "browser_transport", "read"); e != nil {
		return nil, e
	}
	windows, e := a.dispatcher.Windows(ctx)
	if e != nil {
		return nil, e
	}
	out := []browsergrant.DispatchWindow{}
	for _, v := range windows {
		w, ok := s.host.windows[v.Instance]
		host := HostIdentity{Instance: v.Instance, Lease: w.Lease}
		if ok && s.pairingWindow(host) && windowGeneration(host) == v.Window {
			out = append(out, v)
		}
	}
	return out, nil
}
