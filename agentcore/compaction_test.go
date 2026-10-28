package agentcore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func compactRuntime(t *testing.T, window int, fail bool) (*Runtime, *int) {
	t.Helper()
	calls := new(int)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		*calls++
		var b struct {
			Messages []Message `json:"messages"`
			Tools    []any     `json:"tools"`
			Limit    int       `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(q.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		if len(b.Tools) != 0 {
			t.Error("summary received executable tools")
		}
		if b.Limit <= 0 {
			t.Error("summary output unbounded")
		}
		if len(b.Messages) == 0 || estimateRequest(b.Messages[0].Content, b.Messages[1:], nil)+b.Limit > window {
			t.Error("serialized summary request plus output exceeds configured window")
		}
		if fail {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Goal: keep the decision and finish the work."},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":12}}`))
	}))
	t.Cleanup(s.Close)
	c := config(s.URL)
	p := c.Profiles["mini"]
	p.ContextWindowTokens = window
	c.Profiles["mini"] = p
	r, e := New(c, "", gate{})
	if e != nil {
		t.Fatal(e)
	}
	return r, calls
}
func historyTurns(n, size int) []Message {
	var h []Message
	for i := 0; i < n; i++ {
		h = append(h, Message{Role: "user", Content: strings.Repeat("u", size)}, Message{Role: "assistant", Content: strings.Repeat("a", size)})
	}
	return append(h, Message{Role: "user", Content: "Current request stays verbatim."})
}
func TestCompactionBoundaryAndRecentTurns(t *testing.T) {
	r, calls := compactRuntime(t, 16000, false)
	h := historyTurns(2, 100)
	b, _ := r.contextBudget("mini", r.config.Profiles["mini"])
	estimate := estimateRequest(r.contextSystem(), h, nil)
	r.config.Compaction.ThresholdPercent = float64(estimate+1) * 100 / float64(b.WindowTokens)
	out, state, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil || *calls != 0 || state.Through != 0 || !reflect.DeepEqual(out, h) {
		t.Fatal(e, state, *calls)
	}
	// At the boundary a summary is required, rather than waiting one more turn.
	r.config.Compaction.ThresholdPercent = float64(estimate) * 100 / float64(b.WindowTokens)
	r.config.Compaction.KeepRecentTurns = 1
	_, _, e = r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil || *calls == 0 {
		t.Fatal("boundary did not trigger summary", e, *calls)
	}
	r.config.Compaction.KeepRecentTurns = 0
	r.config.Compaction.ThresholdPercent = 70
	h = historyTurns(12, 600)
	original := append([]Message(nil), h...)
	out, state, e = r.CompactContext(context.Background(), RunRequest{Actor: "test", RunID: "turn"}, nil, h, ContextState{})
	if e != nil {
		t.Fatal(e)
	}
	if *calls == 0 || state.Through != 18 || !reflect.DeepEqual(h, original) || !reflect.DeepEqual(out[1:], h[state.Through:]) {
		t.Fatalf("state=%+v calls=%d", state, *calls)
	}
	if !validContextState(state, h) {
		t.Fatal("invalid persisted cache")
	}
	if len(r.Usage()) == 0 || !strings.Contains(r.Usage()[0].RunID, "/compact") {
		t.Fatal("summary not metered")
	}
}
func TestCompactionRestartAndSmallerModelSwitch(t *testing.T) {
	r, calls := compactRuntime(t, 16000, false)
	h := historyTurns(12, 600)
	_, state, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(state)
	var restored ContextState
	json.Unmarshal(raw, &restored)
	n := *calls
	_, _, e = r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, restored)
	if e != nil || *calls != n {
		t.Fatal("restart cache not reused", e, *calls, n)
	}
	p := r.config.Profiles["mini"]
	p.ContextWindowTokens = 4000
	r.config.Profiles["small"] = p
	out, next, e := r.CompactContext(context.Background(), RunRequest{Actor: "test", Profile: "small"}, nil, h, restored)
	if e != nil {
		t.Fatal(e)
	}
	if next.Through <= state.Through || *calls <= n || !reflect.DeepEqual(out[len(out)-1], h[len(h)-1]) {
		t.Fatal("smaller switch did not preserve current request", next, *calls)
	}
}
func TestCompactionFailureAndEditedPrefixNeverLoseHistory(t *testing.T) {
	r, _ := compactRuntime(t, 16000, true)
	h := historyTurns(12, 600)
	original := append([]Message(nil), h...)
	prior := ContextState{Version: 1, Through: 2, PrefixSHA256: prefixDigest(h[:2]), Summary: "Old summary"}
	_, got, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, prior)
	if e == nil || !reflect.DeepEqual(got, prior) || !reflect.DeepEqual(original, h) {
		t.Fatal("failure changed data", e)
	}
	h[0].Content = "edited"
	if validContextState(prior, h) {
		t.Fatal("stale cache accepted")
	}
}
func TestCompactionUnknownCapacityAndOversizedCurrentTurn(t *testing.T) {
	r, _ := compactRuntime(t, 0, false)
	_, _, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, historyTurns(1, 10), ContextState{})
	if e == nil || !strings.Contains(e.Error(), "context_window_tokens") {
		t.Fatal(e)
	}
	p := r.config.Profiles["mini"]
	p.ContextWindowTokens = 2048
	r.config.Profiles["mini"] = p
	h := []Message{{Role: "user", Content: strings.Repeat("x", 3000)}}
	_, _, e = r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e == nil || !strings.Contains(e.Error(), "shorten") {
		t.Fatal(e)
	}
	r.config.Compaction.Disabled = true
	out, _, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil || !reflect.DeepEqual(out, h) {
		t.Fatal(e)
	}
}
func TestCompactionConfigAndOutputReservation(t *testing.T) {
	for _, pct := range []float64{-1, 100, 101} {
		c := config("http://localhost:1")
		c.Compaction.ThresholdPercent = pct
		if _, e := New(c, "", gate{}); e == nil {
			t.Fatal(pct)
		}
	}
	r, _ := compactRuntime(t, 4096, false)
	p := r.config.Profiles["mini"]
	p.MaxCompletionTokens = 2000
	r.config.Compaction.ThresholdPercent = 90
	b, e := r.contextBudget("mini", p)
	if e != nil || b.TriggerTokens != 2096 {
		t.Fatal(b, e)
	}
}
func TestContinuationGuardCountsToolSchemasAndResults(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"read","arguments":"{}"}}]}}]}`))
	}))
	defer s.Close()
	c := config(s.URL)
	p := c.Profiles["mini"]
	p.ContextWindowTokens = 2048
	c.Profiles["mini"] = p
	r, _ := New(c, "", gate{})
	_, e := r.Run(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "read"}}}, []Tool{{Name: "read", Execute: func(context.Context, json.RawMessage) (string, error) { return strings.Repeat("x", 3000), nil }}})
	if e == nil || calls != 1 {
		t.Fatal("oversized continuation sent", calls, e)
	}
}

func TestCompactionConsumesOversizedHistoricalMessageInChunks(t *testing.T) {
	r, calls := compactRuntime(t, 8000, false)
	h := []Message{{Role: "user", Content: strings.Repeat("historical界", 3000)}, {Role: "assistant", Content: "old response"}, {Role: "user", Content: "new"}}
	original := append([]Message(nil), h...)
	out, state, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil {
		t.Fatal(e)
	}
	if *calls < 2 || state.Through != 2 || !reflect.DeepEqual(original, h) || out[len(out)-1].Content != "new" {
		t.Fatal("historical chunks not summarized safely", state, *calls)
	}
}
func TestFallbackUsesItsOwnWindowAndPinnedContextCounts(t *testing.T) {
	r, _ := compactRuntime(t, 16000, false)
	pool := r.config.Pools["openai"]
	pool.MonthlyTargetUSD = 1
	pool.FallbackProfile = "local"
	r.config.Pools["openai"] = pool
	r.config.Pools["local"] = Pool{BaseURL: pool.BaseURL}
	r.config.Profiles["local"] = Profile{Pool: "local", Model: "small", ContextWindowTokens: 2048}
	r.record(UsageRecord{Time: r.Now(), Pool: "openai", EstimatedUSD: 2})
	_, _, e := r.CompactContext(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "system", Content: strings.Repeat("x", 2000)}}}, nil, []Message{{Role: "user", Content: "new"}}, ContextState{})
	if e == nil || !strings.Contains(e.Error(), "local") {
		t.Fatal("fallback/pinned budget ignored", e)
	}
}

func TestConfiguredContextWindowPreservesExistingImageLane(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"screen","arguments":"{}"}}]}}]}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"saw image"}}]}`))
	}))
	defer s.Close()
	c := config(s.URL)
	p := c.Profiles["mini"]
	p.ContextWindowTokens = 16000
	p.Vision = true
	c.Profiles["mini"] = p
	r, _ := New(c, "", gate{})
	out, e := r.Run(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "inspect"}}}, []Tool{{Name: "screen", ExecuteResult: func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Text: "screen", Images: []Image{{MIMEType: "image/png", Data: imagePNG(t)}}}, nil
	}}})
	if e != nil || calls != 2 || out.Text != "saw image" || len(out.Warnings) == 0 {
		t.Fatal("image lane regressed", out, e, calls)
	}
}

func TestCompactionRejectsIncompleteSummaryWithoutChangingCache(t *testing.T) {
	for _, reason := range []string{"length", "content_filter", "incomplete", ""} {
		t.Run("reason_"+reason, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				calls++
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "Nonempty but incomplete."}, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 10}})
			}))
			defer server.Close()
			c := config(server.URL)
			p := c.Profiles["mini"]
			p.ContextWindowTokens = 16000
			c.Profiles["mini"] = p
			r, _ := New(c, "", gate{})
			h := historyTurns(12, 600)
			original := append([]Message(nil), h...)
			prior := ContextState{Version: 1, Through: 2, PrefixSHA256: prefixDigest(h[:2]), Summary: "Previous complete summary"}
			_, next, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, prior)
			if e == nil || !strings.Contains(e.Error(), "incomplete") || calls != 1 || !reflect.DeepEqual(next, prior) || !reflect.DeepEqual(h, original) {
				t.Fatal("incomplete completion changed cache/history", e, next, calls)
			}
			if len(r.Usage()) != 1 {
				t.Fatal("rejected summary usage lost")
			}
		})
	}
}

func TestCompactionEscapedHistoricalMessageFitsFinalSerializedRequests(t *testing.T) {
	r, calls := compactRuntime(t, 8000, false)
	h := []Message{{Role: "user", Content: strings.Repeat("\\\"\n", 12000)}, {Role: "assistant", Content: "Old answer"}, {Role: "user", Content: "Keep current request"}}
	original := append([]Message(nil), h...)
	out, state, e := r.CompactContext(context.Background(), RunRequest{Actor: "test"}, nil, h, ContextState{})
	if e != nil {
		t.Fatal(e)
	}
	if *calls < 2 || state.Through != 2 || !reflect.DeepEqual(h, original) || out[len(out)-1].Content != "Keep current request" {
		t.Fatal("escaped historical message not safely summarized", state, *calls)
	}
}
