package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type draftGate struct{ denyTool bool }

func (g *draftGate) Enforce(_ context.Context, q contract.Request) error {
	if g.denyTool && q.Obj == "agent/tool/fixture" {
		return errors.New("tool revoked")
	}
	return nil
}
func preparedFixture(t *testing.T) (*Runtime, RunRequest, []Tool, *int, *int, *draftGate) {
	t.Helper()
	calls, executions := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"one","type":"function","function":{"name":"fixture","arguments":"{}"}}]}}]}`)
		} else {
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"done"}}]}`)
		}
	}))
	t.Cleanup(server.Close)
	gate := &draftGate{}
	runtime, e := New(Config{DefaultProfile: "local", MaxSteps: 4, Timezone: "UTC", Instructions: "fixture instructions", Pools: map[string]Pool{"p": {BaseURL: server.URL}}, Profiles: map[string]Profile{"local": {Pool: "p", Model: "fixture"}}}, filepath.Join(t.TempDir(), "usage.json"), gate)
	if e != nil {
		t.Fatal(e)
	}
	q := RunRequest{Actor: "test", RunID: "hold", Messages: []Message{{Role: "system", Content: "full context"}, {Role: "user", Content: "final question"}}}
	tools := []Tool{{Name: "fixture", Parameters: map[string]any{"type": "object"}, Execute: func(context.Context, json.RawMessage) (string, error) { executions++; return "evidence", nil }}}
	return runtime, q, tools, &calls, &executions, gate
}
func TestPrepareRunsNoToolsAndReleaseResumesOnce(t *testing.T) {
	r, q, tools, calls, executions, _ := preparedFixture(t)
	p, e := r.Prepare(context.Background(), q, tools)
	if e != nil {
		t.Fatal(e)
	}
	if *calls != 1 || *executions != 0 {
		t.Fatal("speculation executed a tool")
	}
	q.RunID = "real-release"
	out, e := r.RunPrepared(context.Background(), q, tools, p)
	if e != nil || out.Text != "done" {
		t.Fatal(e, out)
	}
	if *calls != 2 || *executions != 1 {
		t.Fatal("first decision repeated or tool missing", *calls, *executions)
	}
	if _, e = r.RunPrepared(context.Background(), q, tools, p); !errors.Is(e, ErrStalePrepared) {
		t.Fatal("reused preparation accepted", e)
	}
	if *executions != 1 {
		t.Fatal("tool executed twice")
	}
}
func TestPreparedTextContextModelAndSchemasAreBound(t *testing.T) {
	for _, change := range []string{"text", "context", "model", "schema", "expiry"} {
		t.Run(change, func(t *testing.T) {
			r, q, tools, _, executions, _ := preparedFixture(t)
			p, e := r.Prepare(context.Background(), q, tools)
			if e != nil {
				t.Fatal(e)
			}
			switch change {
			case "text":
				q.Messages[1].Content = "a different final question"
			case "context":
				q.Messages[0].Content = "changed context"
			case "model":
				profile := r.config.Profiles["local"]
				profile.Model = "different"
				r.config.Profiles["local"] = profile
			case "schema":
				tools[0].Description = "changed contract"
			case "expiry":
				r.Now = func() time.Time { return time.Now().Add(time.Minute) }
			}
			if _, e = r.RunPrepared(context.Background(), q, tools, p); !errors.Is(e, ErrStalePrepared) {
				t.Fatal("stale preparation accepted", e)
			}
			if *executions != 0 {
				t.Fatal("stale tool ran")
			}
		})
	}
}
func TestPreparedToolRechecksCurrentPolicy(t *testing.T) {
	r, q, tools, _, executions, gate := preparedFixture(t)
	p, e := r.Prepare(context.Background(), q, tools)
	if e != nil {
		t.Fatal(e)
	}
	gate.denyTool = true
	if _, e = r.RunPrepared(context.Background(), q, tools, p); e == nil {
		t.Fatal("revocation ignored")
	}
	if *executions != 0 {
		t.Fatal("revoked tool ran")
	}
}
