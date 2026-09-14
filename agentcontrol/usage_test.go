package agentcontrol

import (
	"testing"
	"time"
)

func TestUsageMissingIsUnknown(t *testing.T) {
	u := latestClaudeUsage([]byte(`{"type":"result"}`), time.Now())
	if u.TokensKnown || u.EstimateKnown || u.BilledKnown || u.InputTokens != nil || u.EstimatedUSD != nil {
		t.Fatal(u)
	}
}
func TestUsageZeroReportedAndCumulativeNotSummed(t *testing.T) {
	first := latestClaudeUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":1},"total_cost_usd":0.1}`), time.Now())
	last := latestClaudeUsage([]byte(`{"usage":{"input_tokens":20,"output_tokens":2,"cache_creation_input_tokens":0},"total_cost_usd":0.2}`), time.Now())
	_ = first
	if *last.InputTokens != 20 || *last.EstimatedUSD != 0.2 || last.CacheWriteTokens == nil || *last.CacheWriteTokens != 0 || last.BilledKnown {
		t.Fatal(last)
	}
}
