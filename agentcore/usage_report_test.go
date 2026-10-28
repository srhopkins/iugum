package agentcore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUsageStatusWindowAndUnknowns(t *testing.T) {
	start := "2026-10-21T00:00:00-07:00"
	end := "2026-10-24T00:00:00-07:00"
	at, _ := time.Parse(time.RFC3339, start)
	r := &Runtime{records: []UsageRecord{
		{Time: at.Add(-time.Nanosecond), Pool: "before", UsageKnown: true, PriceKnown: true, InputTokens: 999},
		{Time: at, Pool: "z", UsageKnown: true, PriceKnown: true, InputTokens: 10, OutputTokens: 2, EstimatedUSD: .03},
		{Time: at.Add(time.Hour), Pool: "a", UsageKnown: true, InputTokens: 20, OutputTokens: 3, EstimatedUSD: 999},
		{Time: at.Add(2 * time.Hour), Pool: "a", PriceKnown: true, InputTokens: 999, EstimatedUSD: 999},
		{Time: at.Add(72*time.Hour - time.Nanosecond), Pool: "a"},
		{Time: at.Add(72 * time.Hour), Pool: "after", UsageKnown: true, InputTokens: 999},
	}}
	got, err := r.UsageStatus(start, end)
	if err != nil {
		t.Fatal(err)
	}
	want := UsageTotals{Calls: 4, InputTokens: 30, OutputTokens: 5, TotalTokens: 35, MissingUsageCalls: 2, MissingPriceCalls: 2, EstimatedUSD: .03, CostIncomplete: true}
	if got.Totals != want {
		t.Fatalf("totals %#v, want %#v", got.Totals, want)
	}
	if len(got.Pools) != 2 || got.Pools[0].Pool != "a" || got.Pools[1].Pool != "z" || got.Pools[1].CostIncomplete {
		t.Fatalf("pools %#v", got.Pools)
	}
	if got.Start.Location() != time.UTC || got.Start.Hour() != 7 {
		t.Fatalf("UTC boundary %s", got.Start)
	}
	empty, err := r.UsageStatus("2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z")
	if err != nil || empty.Totals != (UsageTotals{}) || empty.Pools == nil || len(empty.Pools) != 0 {
		t.Fatalf("empty %#v %v", empty, err)
	}
}
func TestUsageStatusValidationAndFreshLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	r := &Runtime{path: path}
	for _, tc := range [][2]string{{"", "2026-01-02T00:00:00Z"}, {"2026-01-01", "2026-01-02"}, {"2026-01-01T00:00:00", "2026-01-02T00:00:00Z"}, {"2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z"}, {"2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}, {"2026-01-01T00:00:00Z", "bad"}} {
		if _, err := r.UsageStatus(tc[0], tc[1]); err == nil {
			t.Fatalf("accepted %v", tc)
		}
	}
	start, end := "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
	if err := os.WriteFile(path, []byte(`[{"time":"2026-01-01T00:00:00Z","pool":"local","usage_known":true,"price_known":true,"input_tokens":5}]`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := r.UsageStatus(start, end)
	if err != nil || got.Totals.InputTokens != 5 || got.Totals.CostIncomplete {
		t.Fatalf("fresh %#v %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UsageStatus(start, end); err == nil {
		t.Fatal("corrupt ledger silently accepted")
	}
}

func TestUsageStatusRejectsLenientRFC3339(t *testing.T) {
	r := &Runtime{}
	for _, stamp := range []string{"2026-10-24T00:00:00+24:00", "2026-10-24T00:00:00+00:60", "2026-10-24T0:00:00Z", "2026-10-24T00:00:00,1Z", "2026-10-24T24:00:00Z", "2026-02-30T00:00:00Z"} {
		for _, window := range [][2]string{{stamp, "2027-01-01T00:00:00Z"}, {"2025-01-01T00:00:00Z", stamp}} {
			if _, err := r.UsageStatus(window[0], window[1]); err == nil {
				t.Fatal("accepted malformed timestamp", window)
			}
		}
	}
}
