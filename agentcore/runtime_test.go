package agentcore

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type gate struct{ deny string }

func (g gate) Enforce(_ context.Context, r contract.Request) error {
	if strings.Contains(r.Obj, g.deny) && g.deny != "" {
		return contract.Denied{Req: r}
	}
	return nil
}
func config(url string) Config {
	return Config{DefaultProfile: "mini", Timezone: "America/Phoenix", Pools: map[string]Pool{"openai": {BaseURL: url, MonthlyTargetUSD: 50}}, Profiles: map[string]Profile{"mini": {Pool: "openai", Model: "test", InputUSDPerMillion: 1, OutputUSDPerMillion: 2}}}
}
func TestToolLoopFreshTimeAndUsage(t *testing.T) {
	var stamps []string
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		stamps = append(stamps, body.Messages[0].Content)
		calls++
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"search","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		} else {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"found"}}],"usage":{"prompt_tokens":20,"completion_tokens":5}}`))
		}
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "usage.json")
	r, e := New(config(s.URL), path, gate{})
	if e != nil {
		t.Fatal(e)
	}
	n := 0
	r.Now = func() time.Time { n++; return time.Date(2026, 9, 10, 12, 0, n, 0, time.UTC) }
	authorized := false
	out, e := r.Run(context.Background(), RunRequest{Actor: "chief", RunID: "run", Messages: []Message{{Role: "user", Content: "find"}}}, []Tool{{Name: "search", Parameters: map[string]any{"type": "object"}, Authorize: func(context.Context, json.RawMessage) error { authorized = true; return nil }, Execute: func(context.Context, json.RawMessage) (string, error) { return "evidence", nil }}})
	if e != nil {
		t.Fatal(e)
	}
	if out.Text != "found" || !authorized || len(stamps) != 2 || stamps[0] == stamps[1] {
		t.Fatalf("unexpected result %+v %v", out, stamps)
	}
	loaded, e := New(config(s.URL), path, gate{})
	if e != nil || len(loaded.Usage()) != 2 {
		t.Fatalf("ledger: %v", e)
	}
	if !loaded.Usage()[0].UsageKnown || loaded.Usage()[0].EstimatedUSD <= 0 {
		t.Fatal("usage not tracked")
	}
}
func TestPolicyAndCredentials(t *testing.T) {
	r, e := New(config("https://example.invalid/v1"), "", gate{deny: "agent/model"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = r.Run(context.Background(), RunRequest{Actor: "chief"}, nil)
	if e == nil || !strings.Contains(e.Error(), "policy denied") {
		t.Fatal(e)
	}
	c := config("https://example.invalid/v1")
	p := c.Pools["openai"]
	p.APIKeyEnv = "IUGUM_TEST_MISSING_KEY"
	c.Pools["openai"] = p
	t.Setenv(p.APIKeyEnv, "")
	r, _ = New(c, "", gate{})
	_, e = r.Run(context.Background(), RunRequest{Actor: "chief"}, nil)
	if e == nil || !strings.Contains(e.Error(), "not set") {
		t.Fatal(e)
	}
}
func TestSoftTargetFallbackAndUnknownUsage(t *testing.T) {
	c := config("http://localhost:1")
	p := c.Pools["openai"]
	p.FallbackProfile = "local"
	c.Pools["openai"] = p
	c.Pools["local"] = Pool{BaseURL: "http://localhost:2"}
	c.Profiles["local"] = Profile{Pool: "local", Model: "small"}
	r, _ := New(c, "", gate{})
	r.record(UsageRecord{Time: time.Now(), Pool: "openai", EstimatedUSD: 51})
	name, w := r.route("")
	if name != "local" || len(w) == 0 {
		t.Fatal(name, w)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := r.Run(ctx, RunRequest{Actor: "chief"}, nil)
	if e != context.Canceled {
		t.Fatal(e)
	}
}
func TestUnknownUsage(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer s.Close()
	r, _ := New(config(s.URL), "", gate{})
	_, e := r.Run(context.Background(), RunRequest{Actor: "chief"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if r.Usage()[0].UsageKnown {
		t.Fatal("missing usage must be unknown")
	}
}

func TestDeniedToolNeverExecutes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"write","arguments":"{}"}}]}}]}`))
	}))
	defer s.Close()
	r, _ := New(config(s.URL), "", gate{deny: "agent/tool"})
	ran := false
	_, err := r.Run(context.Background(), RunRequest{Actor: "chief"}, []Tool{{Name: "write", Execute: func(context.Context, json.RawMessage) (string, error) { ran = true; return "", nil }}})
	if err == nil || ran {
		t.Fatalf("denied tool executed: %v %v", ran, err)
	}
}

func TestStepLimit(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"read","arguments":"{}"}}]}}]}`))
	}))
	defer s.Close()
	c := config(s.URL)
	c.MaxSteps = 2
	r, _ := New(c, "", gate{})
	_, err := r.Run(context.Background(), RunRequest{Actor: "chief"}, []Tool{{Name: "read", Execute: func(context.Context, json.RawMessage) (string, error) { return "ok", nil }}})
	if err == nil || !strings.Contains(err.Error(), "maximum model steps") || calls != 2 {
		t.Fatal(calls, err)
	}
}
