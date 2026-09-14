package agentcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUsageLedgerWorker(t *testing.T) {
	path := os.Getenv("IUGUM_TEST_LEDGER")
	if path == "" {
		return
	}
	r := &Runtime{path: path}
	for n := 0; n < 20; n++ {
		if e := r.record(UsageRecord{Pool: "shared", RunID: os.Getenv("IUGUM_TEST_WORKER")}); e != nil {
			t.Fatal(e)
		}
	}
}
func TestSharedLedgerAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for _, worker := range []string{"parent", "clone"} {
		wg.Add(1)
		go func(w string) {
			defer wg.Done()
			cmd := exec.Command(exe, "-test.run=^TestUsageLedgerWorker$")
			cmd.Env = append(os.Environ(), "IUGUM_TEST_LEDGER="+path, "IUGUM_TEST_WORKER="+w)
			if b, e := cmd.CombinedOutput(); e != nil {
				t.Errorf("worker: %s %v", b, e)
			}
		}(worker)
	}
	wg.Wait()
	r := &Runtime{path: path}
	records, e := r.UsageFresh()
	if e != nil || len(records) != 40 {
		t.Fatal(len(records), e)
	}
}
func TestRoutingTimezoneAlertsAndCycles(t *testing.T) {
	now := time.Date(2026, 10, 1, 2, 0, 0, 0, time.UTC)
	r := &Runtime{Now: func() time.Time { return now }, config: Config{Timezone: "America/Phoenix", DefaultProfile: "mini", Profiles: map[string]Profile{"mini": {Pool: "openai"}, "local": {Pool: "local"}}, Pools: map[string]Pool{"openai": {MonthlyTargetUSD: 50, FallbackProfile: "local"}, "local": {MonthlyTargetUSD: 1, FallbackProfile: "mini"}}}, records: []UsageRecord{{Time: now, Pool: "openai", EstimatedUSD: 50, UsageKnown: true, PriceKnown: true}, {Time: now, Pool: "local", EstimatedUSD: 1}}}
	d, e := r.RoutingStatus("")
	if e != nil || d.SelectedProfile != "local" || len(d.Warnings) < 3 {
		t.Fatal(d, e)
	}
	for _, s := range d.Pools {
		if s.Month != "2026-09" {
			t.Fatal(s)
		}
	}
	r.records = r.records[:1]
	r.records[0].EstimatedUSD = 40
	d, e = r.RoutingStatus("")
	if e != nil || d.SelectedProfile != "mini" {
		t.Fatal(d, e)
	}
	for _, s := range d.Pools {
		if s.Pool == "openai" && len(s.Alerts) != 1 {
			t.Fatal(s)
		}
	}
}
func TestCorruptLedgerCannotBeOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	os.WriteFile(path, []byte("invalid"), 0600)
	r := &Runtime{path: path}
	if e := r.record(UsageRecord{}); e == nil {
		t.Fatal("corrupt ledger overwritten")
	}
	b, _ := os.ReadFile(path)
	if string(b) != "invalid" {
		t.Fatal(string(b))
	}
}
