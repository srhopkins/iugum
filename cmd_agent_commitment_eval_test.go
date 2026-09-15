package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/srhopkins/iugum/agentcore"
	"github.com/srhopkins/iugum/agentdesk"
)

// Opt-in behavioral evaluation: uses subscription allowance, but only disposable
// commitment storage and a synthetic read tool. No external account actions.
func TestChiefCurrentTaskListEvaluation(t *testing.T) {
	model := os.Getenv("IUGUM_CHIEF_EVAL_MODEL")
	if model == "" {
		t.Skip("set IUGUM_CHIEF_EVAL_MODEL to run the isolated subscription evaluation")
	}
	instructions, err := os.ReadFile("examples/chief/soul.md")
	if err != nil {
		t.Fatal(err)
	}
	desk, err := agentdesk.New(agentdesk.Config{Name: "test", DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	current := "I need to\n- make sure my email PRs merge for Acme\n- merge Beacon article changes\n- respond to Morgan; I am thinking about ending support, but need a tactful response either way\n- check outstanding deliverables to Casey my bookkeeper and send the state tax letter; I may owe money or a report"
	tools := nativeCommitmentTools(desk, current)
	reads := 0
	tools = append(tools, agentcore.Tool{Name: "search", Description: "Read indexed prior work for a task. Data may be incomplete.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]string{"type": "string"}}, "required": []string{"query"}}, Execute: func(context.Context, json.RawMessage) (string, error) {
		reads++
		return "No matching evidence. Live account access is unavailable in this evaluation.", nil
	}})
	cfg := agentcore.Config{Instructions: string(instructions), Timezone: "America/Phoenix", MaxSteps: 8, DefaultProfile: "test", Pools: map[string]agentcore.Pool{"test": {BaseURL: "http://127.0.0.1:1"}}, Profiles: map[string]agentcore.Profile{"test": {Pool: "test", Model: "unused"}}}
	runtime, err := agentcore.New(cfg, "", routingTestPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := runtime.RunSubscriptionTools(ctx, agentcore.RunRequest{Actor: "test", Messages: []agentcore.Message{
		{Role: "user", Content: "What was my last coding session?"},
		{Role: "assistant", Content: "The last recorded session was about email sender domains."},
		{Role: "user", Content: current},
	}}, "codex", model, tools)
	if err != nil {
		t.Fatal(err)
	}
	state := desk.Context()
	for _, subject := range []string{"Acme", "Beacon", "Morgan", "Casey"} {
		if !strings.Contains(state, subject) {
			t.Errorf("missing saved task for %s", subject)
		}
	}
	if reads == 0 {
		t.Error("no research attempted after capture")
	}
	t.Log("Saved commitments:", state)
	t.Log("Reply:", result.Text)
}
