package agentcore

import (
	"fmt"
	"regexp"
	"sort"
	"time"
)

// UsageTotals counts only reported tokens and prices; unavailable data is explicit.
type UsageTotals struct {
	Calls             int     `json:"calls"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	MissingUsageCalls int     `json:"missing_usage_calls"`
	MissingPriceCalls int     `json:"missing_price_calls"`
	EstimatedUSD      float64 `json:"estimated_usd"`
	CostIncomplete    bool    `json:"cost_incomplete"`
}
type PoolUsage struct {
	Pool string `json:"pool"`
	UsageTotals
}
type UsageReport struct {
	Start  time.Time   `json:"start_inclusive"`
	End    time.Time   `json:"end_exclusive"`
	Totals UsageTotals `json:"totals"`
	Pools  []PoolUsage `json:"pools"`
}

var usageTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// UsageStatus reads the fresh ledger for [start,end). RFC3339 timestamps must
// include Z or a numeric offset; relative/calendar dates are resolved by callers.
func (r *Runtime) UsageStatus(start, end string) (UsageReport, error) {
	report := UsageReport{Pools: []PoolUsage{}}
	var err error
	if !usageTimestamp.MatchString(start) || !usageTimestamp.MatchString(end) {
		return report, fmt.Errorf("start and end must be RFC3339 timestamps with valid timezone offsets")
	}
	if report.Start, err = time.Parse(time.RFC3339Nano, start); err != nil {
		return report, fmt.Errorf("start must be an RFC3339 timestamp with timezone: %w", err)
	}
	if report.End, err = time.Parse(time.RFC3339Nano, end); err != nil {
		return report, fmt.Errorf("end must be an RFC3339 timestamp with timezone: %w", err)
	}
	if !report.Start.Before(report.End) {
		return report, fmt.Errorf("start must be before end (start inclusive, end exclusive)")
	}
	report.Start, report.End = report.Start.UTC(), report.End.UTC()
	records, err := r.UsageFresh()
	if err != nil {
		return report, err
	}
	pools := map[string]*UsageTotals{}
	add := func(t *UsageTotals, u UsageRecord) {
		t.Calls++
		if u.UsageKnown {
			t.InputTokens += int64(u.InputTokens)
			t.OutputTokens += int64(u.OutputTokens)
			t.TotalTokens += int64(u.InputTokens) + int64(u.OutputTokens)
		} else {
			t.MissingUsageCalls++
		}
		if !u.PriceKnown {
			t.MissingPriceCalls++
		}
		if u.UsageKnown && u.PriceKnown {
			t.EstimatedUSD += u.EstimatedUSD
		}
		t.CostIncomplete = t.MissingUsageCalls > 0 || t.MissingPriceCalls > 0
	}
	for _, u := range records {
		if u.Time.Before(report.Start) || !u.Time.Before(report.End) {
			continue
		}
		if pools[u.Pool] == nil {
			pools[u.Pool] = &UsageTotals{}
		}
		add(pools[u.Pool], u)
		add(&report.Totals, u)
	}
	for pool, totals := range pools {
		report.Pools = append(report.Pools, PoolUsage{Pool: pool, UsageTotals: *totals})
	}
	sort.Slice(report.Pools, func(i, j int) bool { return report.Pools[i].Pool < report.Pools[j].Pool })
	return report, nil
}
