// Test-only stdio transport for joined Node/Go source fixtures. No listener,
// providers, browser APIs or user data. All HTTP goes through actual Handler.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/srhopkins/iugum/agentplatform"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
)

const firstInstance = "11111111-1111-4111-8111-111111111111"

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "joined source helper:", e)
		os.Exit(1)
	}
}
func run() error {
	dir, e := os.MkdirTemp("", "iugum-browser-joined-source-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	gate, e := policy.New("", "")
	if e != nil {
		return e
	}
	s, e := agentplatform.New(context.Background(), agentplatform.Config{DataDir: dir, HostTools: true, BrowserGrants: true, BrowserActor: "fixture", BrowserPolicy: gate, Check: func(ctx context.Context, obj, act string) error {
		return gate.Enforce(ctx, contract.Request{Sub: "fixture", Obj: obj, Act: act})
	}})
	if e != nil {
		return e
	}
	defer s.Close()
	token, e := os.ReadFile(filepath.Join(dir, "control-token"))
	if e != nil {
		return e
	}
	allowed := map[string]bool{"/api/host/register": true}
	for _, name := range []string{"offer", "commit", "reveal", "confirm", "companion-confirm", "status", "cancel"} {
		allowed["/api/host/browser/bootstrap/"+name] = true
	}
	for _, name := range []string{"grant", "observation", "result", "poll", "clock-probe", "status", "cancel", "mcp"} {
		allowed["/api/host/browser/"+name] = true
	}
	request := func(path string, body any) (map[string]any, error) {
		if !allowed[path] {
			return nil, fmt.Errorf("fixture route unavailable")
		}
		b, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+string(token))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-06-18")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var value any
		if e = json.Unmarshal(w.Body.Bytes(), &value); e != nil {
			value = w.Body.String()
		}
		return map[string]any{"status": w.Code, "body": value}, nil
	}
	instance := firstInstance
	rotations := 0
	register := func() (any, error) {
		v, e := request("/api/host/register", map[string]any{"instance": instance, "label": "Joined source fixture", "capabilities": []string{"inspect"}})
		if e != nil {
			return nil, e
		}
		if v["status"] != 200 {
			return nil, fmt.Errorf("fixture registration failed")
		}
		return v["body"], nil
	}
	host, e := register()
	if e != nil {
		return e
	}
	out := json.NewEncoder(os.Stdout)
	if e = out.Encode(map[string]any{"ready": true, "host": host}); e != nil {
		return e
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	for scanner.Scan() {
		var q struct {
			Operation string          `json:"operation"`
			Body      json.RawMessage `json:"body"`
		}
		if e = json.Unmarshal(scanner.Bytes(), &q); e != nil {
			return e
		}
		var response any
		switch q.Operation {
		case "request":
			var v struct {
				Path string `json:"path"`
				Body any    `json:"body"`
			}
			if e = json.Unmarshal(q.Body, &v); e != nil {
				return e
			}
			response, e = request(v.Path, v.Body)
		case "agent-call":
			var v struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if e = json.Unmarshal(q.Body, &v); e != nil {
				return e
			}
			if v.Name != "browser_call" && v.Name != "browser_status" && v.Name != "browser_windows" {
				return fmt.Errorf("fixed browser MCP tool required")
			}
			response, e = request("/api/host/browser/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": v.Name, "arguments": v.Arguments}})
		case "rotate-window":
			rotations++
			instance = fmt.Sprintf("22222222-2222-4222-8222-%012d", rotations)
			host, e = register()
			response = map[string]any{"host": host}
		case "close":
			return nil
		default:
			return fmt.Errorf("unsupported source fixture operation")
		}
		if e != nil {
			return e
		}
		if e = out.Encode(response); e != nil {
			return e
		}
	}
	if e = scanner.Err(); e != nil && e != io.EOF {
		return e
	}
	return nil
}
