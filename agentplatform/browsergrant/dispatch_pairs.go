package browsergrant

import (
	"context"
	"errors"
	"github.com/srhopkins/iugum/contract"
)

// DispatchWindow contains no lease, key or browser-provided readiness assertion.
type DispatchWindow struct {
	Pair        string `json:"pair"`
	Instance    string `json:"instance"`
	Window      string `json:"-"`
	Client      string `json:"client"`
	Document    string `json:"document"`
	TabID       int64  `json:"tabId"`
	Origin      string `json:"origin"`
	Phase       string `json:"phase"`
	Observation string `json:"observation,omitempty"`
	Expires     int64  `json:"expires"`
}

func (d *Dispatcher) Windows(ctx context.Context) ([]DispatchWindow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if d.closed || a.closed || !d.health.healthy || !a.health.healthy {
		return nil, errors.New("browser authority unavailable")
	}
	if err := a.gate.Enforce(ctx, contract.Request{Sub: a.actor, Obj: "agent/browser/windows", Act: "read"}); err != nil {
		return nil, err
	}
	out := []DispatchWindow{}
	for _, p := range a.pairs {
		if p.Expires <= a.now().UnixMilli() {
			continue
		}
		v := DispatchWindow{Pair: p.ID, Instance: p.Instance, Window: p.WindowLease, Client: p.Client, Document: p.Document, TabID: p.TabID, Origin: p.Origin, Phase: "awaiting-first-observation", Expires: p.Expires}
		if oid := d.latest[p.ID]; oid != "" {
			if proof := a.observations[oid]; proof != nil && proof["ready"] == true && proof["expires"].(int64) > a.now().UnixMilli() {
				v.Phase = "fresh-observation"
				v.Observation = oid
			}
		}
		out = append(out, v)
	}
	return out, nil
}
