package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// UsageFresh reads the shared ledger's latest complete atomic snapshot.
func (r *Runtime) UsageFresh() ([]UsageRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path != "" {
		records, e := readLedger(r.path)
		if e != nil {
			return nil, e
		}
		r.records = records
	}
	return append([]UsageRecord{}, r.records...), nil
}

// Usage is retained for callers that cannot handle errors. Routing uses UsageFresh.
func (r *Runtime) Usage() []UsageRecord {
	records, e := r.UsageFresh()
	if e == nil {
		return records
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]UsageRecord{}, r.records...)
}
func readLedger(path string) ([]UsageRecord, error) {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return []UsageRecord{}, nil
	}
	if e != nil {
		return nil, e
	}
	var records []UsageRecord
	if e = json.Unmarshal(b, &records); e != nil {
		return nil, fmt.Errorf("usage ledger: %w", e)
	}
	return records, nil
}

// record uses a stable sidecar lock, reloads under lock, and atomically replaces
// the ledger. Separate clone processes cannot overwrite each other's records.
func (r *Runtime) record(v UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		r.records = append(r.records, v)
		return nil
	}
	if e := os.MkdirAll(filepath.Dir(r.path), 0700); e != nil {
		return e
	}
	lock := flock.New(r.path + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ok, e := lock.TryLockContext(ctx, 20*time.Millisecond)
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("usage ledger lock timeout")
	}
	defer lock.Unlock()
	records, e := readLedger(r.path)
	if e != nil {
		return e
	}
	records = append(records, v)
	data, e := json.MarshalIndent(records, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(r.path), ".usage-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(f.Name(), r.path); e != nil {
		return e
	}
	r.records = records
	return nil
}

type PoolStatus struct {
	Pool             string    `json:"pool"`
	Month            string    `json:"month"`
	EstimatedUSD     float64   `json:"estimated_usd"`
	MonthlyTargetUSD float64   `json:"monthly_target_usd"`
	UnknownRecords   int       `json:"unknown_records"`
	AboveTarget      bool      `json:"above_target"`
	Alerts           []float64 `json:"alerts"`
}
type RouteDecision struct {
	RequestedProfile string       `json:"requested_profile"`
	SelectedProfile  string       `json:"selected_profile"`
	Pools            []PoolStatus `json:"pools"`
	Warnings         []string     `json:"warnings"`
}

// RoutingStatus evaluates configured pool targets in the agent timezone. Targets
// remain soft: an exhausted fallback chain keeps a usable profile with warnings.
func (r *Runtime) RoutingStatus(profile string) (RouteDecision, error) {
	d := RouteDecision{Pools: []PoolStatus{}, Warnings: []string{}}
	if profile == "" {
		profile = r.config.DefaultProfile
	}
	d.RequestedProfile = profile
	d.SelectedProfile = profile
	if _, ok := r.config.Profiles[profile]; !ok {
		return d, fmt.Errorf("unknown profile %q", profile)
	}
	usage, e := r.UsageFresh()
	if e != nil {
		return d, e
	}
	loc, e := time.LoadLocation(r.config.Timezone)
	if e != nil {
		return d, e
	}
	month := r.Now().In(loc).Format("2006-01")
	states := map[string]PoolStatus{}
	var names []string
	for n := range r.config.Pools {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		pool := r.config.Pools[n]
		state := PoolStatus{Pool: n, Month: month, MonthlyTargetUSD: pool.MonthlyTargetUSD, Alerts: []float64{}}
		for _, u := range usage {
			if u.Pool == n && u.Time.In(loc).Format("2006-01") == month {
				state.EstimatedUSD += u.EstimatedUSD
				if !u.UsageKnown || !u.PriceKnown {
					state.UnknownRecords++
				}
			}
		}
		state.AboveTarget = pool.MonthlyTargetUSD > 0 && state.EstimatedUSD >= pool.MonthlyTargetUSD
		fractions := pool.AlertFractions
		if fractions == nil {
			fractions = []float64{.75, .90}
		}
		for _, f := range fractions {
			if f > 0 && f <= 1 && pool.MonthlyTargetUSD > 0 && state.EstimatedUSD >= pool.MonthlyTargetUSD*f {
				state.Alerts = append(state.Alerts, f)
			}
		}
		states[n] = state
		d.Pools = append(d.Pools, state)
	}
	visited := map[string]bool{}
	for {
		visited[profile] = true
		p := r.config.Profiles[profile]
		pool := r.config.Pools[p.Pool]
		state := states[p.Pool]
		d.SelectedProfile = profile
		if state.UnknownRecords > 0 {
			d.Warnings = append(d.Warnings, fmt.Sprintf("Pool %s has %d calls with unknown usage or price; estimated cost is incomplete.", p.Pool, state.UnknownRecords))
		}
		if !state.AboveTarget {
			if len(state.Alerts) > 0 {
				d.Warnings = append(d.Warnings, fmt.Sprintf("Pool %s has used $%.2f of its $%.2f monthly soft target.", p.Pool, state.EstimatedUSD, pool.MonthlyTargetUSD))
			}
			break
		}
		d.Warnings = append(d.Warnings, fmt.Sprintf("Pool %s is above its monthly soft target ($%.2f estimated).", p.Pool, state.EstimatedUSD))
		fallback := pool.FallbackProfile
		if fallback == "" {
			break
		}
		if _, ok := r.config.Profiles[fallback]; !ok {
			d.Warnings = append(d.Warnings, "Configured fallback profile is unavailable: "+fallback)
			break
		}
		if visited[fallback] {
			d.Warnings = append(d.Warnings, "Fallback cycle detected; continuing with "+profile)
			break
		}
		profile = fallback
	}
	return d, nil
}
func (r *Runtime) route(profile string) (string, []string) {
	d, e := r.RoutingStatus(profile)
	if e != nil {
		return profile, []string{e.Error()}
	}
	return d.SelectedProfile, d.Warnings
}
