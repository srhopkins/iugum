package main

import (
	"context"
	"encoding/json"
	"github.com/srhopkins/iugum/agentcore"
)

func nativeUsageTool(runtime *agentcore.Runtime, check func(context.Context, string, string) error) agentcore.Tool {
	return agentcore.Tool{Name: "usage_status", Description: "Read fresh recorded model call and input/output token totals for a requested time window, by pool and overall. Supply RFC3339 start (inclusive) and end (exclusive) with Z or numeric timezone offsets. Resolve relative dates in the user's timezone using the current timestamp; Distinguish a rolling three-day window ending now from three complete calendar days ending at local midnight; state the exact window in the answer. Costs are estimates from configured prices; missing usage and missing prices are separate counts. This is historical usage, not routing budgets. No model calls or price lookup.", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"start": map[string]string{"type": "string", "description": "Inclusive RFC3339 timestamp with timezone"}, "end": map[string]string{"type": "string", "description": "Exclusive RFC3339 timestamp with timezone"}}, "required": []string{"start", "end"}}, Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
		if err := check(ctx, "agent/routing", "read"); err != nil {
			return "", err
		}
		var q struct {
			Start string `json:"start"`
			End   string `json:"end"`
		}
		if err := json.Unmarshal(raw, &q); err != nil {
			return "", err
		}
		report, err := runtime.UsageStatus(q.Start, q.End)
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(report)
		return string(b), err
	}}
}
