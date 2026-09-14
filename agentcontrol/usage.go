package agentcontrol

import (
	"encoding/json"
	"time"
)

// SessionUsage is the latest result's provider-reported snapshot. It is not a
// sum: Claude may report cumulative estimates across turns or a resumed session.
// Pointer fields remain null when the provider did not report them.
type SessionUsage struct {
	Source           string    `json:"source"`
	Scope            string    `json:"scope"`
	CheckedAt        time.Time `json:"checked_at"`
	TokensKnown      bool      `json:"tokens_known"`
	InputTokens      *int64    `json:"input_tokens"`
	OutputTokens     *int64    `json:"output_tokens"`
	CacheReadTokens  *int64    `json:"cache_read_input_tokens"`
	CacheWriteTokens *int64    `json:"cache_creation_input_tokens"`
	EstimateKnown    bool      `json:"estimate_known"`
	EstimatedUSD     *float64  `json:"estimated_usd"`
	BilledKnown      bool      `json:"billed_known"`
	BilledUSD        *float64  `json:"billed_usd"`
}

func latestClaudeUsage(raw []byte, now time.Time) SessionUsage {
	var ev struct {
		Usage *struct {
			Input      *int64 `json:"input_tokens"`
			Output     *int64 `json:"output_tokens"`
			CacheRead  *int64 `json:"cache_read_input_tokens"`
			CacheWrite *int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
		Cost *float64 `json:"total_cost_usd"`
	}
	_ = json.Unmarshal(raw, &ev)
	u := SessionUsage{Source: "claude-subscription", Scope: "latest-result-not-summed", CheckedAt: now.UTC(), EstimatedUSD: ev.Cost, EstimateKnown: ev.Cost != nil}
	if ev.Usage != nil {
		u.InputTokens = ev.Usage.Input
		u.OutputTokens = ev.Usage.Output
		u.CacheReadTokens = ev.Usage.CacheRead
		u.CacheWriteTokens = ev.Usage.CacheWrite
		u.TokensKnown = ev.Usage.Input != nil && ev.Usage.Output != nil
	}
	return u
}
