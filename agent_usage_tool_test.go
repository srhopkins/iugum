package main

import (
	"context"
	"errors"
	"github.com/srhopkins/iugum/agentcore"
	"strings"
	"testing"
)

func TestNativeUsageTool(t *testing.T) {
	r, err := agentcore.New(agentcore.Config{DefaultProfile: "local", Pools: map[string]agentcore.Pool{"local": {BaseURL: "http://127.0.0.1:11434/v1"}}, Profiles: map[string]agentcore.Profile{"local": {Pool: "local", Model: "test", ZeroCost: true}}}, "", routingTestPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	tool := nativeUsageTool(r, func(_ context.Context, obj, action string) error {
		if obj != "agent/routing" || action != "read" {
			t.Fatal(obj, action)
		}
		return nil
	})
	text, err := tool.Execute(context.Background(), []byte(`{"start":"2026-10-21T00:00:00-07:00","end":"2026-10-24T00:00:00-07:00"}`))
	if err != nil || !strings.Contains(text, `"calls":0`) || !strings.Contains(text, `"start_inclusive":"2026-10-21T07:00:00Z"`) {
		t.Fatal(text, err)
	}
	for _, raw := range []string{`{`, `{}`, `{"start":"2026-01-01T00:00:00Z"}`} {
		if _, err := tool.Execute(context.Background(), []byte(raw)); err == nil {
			t.Fatal("accepted", raw)
		}
	}
	denied := errors.New("denied")
	tool = nativeUsageTool(r, func(context.Context, string, string) error { return denied })
	if _, err := tool.Execute(context.Background(), []byte(`{}`)); !errors.Is(err, denied) {
		t.Fatal(err)
	}
	selected, sub := "", ""
	tools := nativeRoutingToolsWithSubscription(r, nativeAgentConfig{}, &selected, &sub, "", func(context.Context, string, string) error { return nil })
	count := 0
	for _, tool := range tools {
		if tool.Name == "usage_status" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("usage tool registrations", count)
	}
}
