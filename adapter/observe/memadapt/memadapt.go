package memadapt

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterObserver("memory", func(map[string]string) (contract.Observer, error) {
		return New(), nil
	})
}

// Observer is an in-process store. sqlite (iugum-9n8) replaces this as the default later.
type Observer struct {
	mu      sync.Mutex
	samples []contract.Sample
	logs    []contract.Log
}

func New() *Observer { return &Observer{} }

func (Observer) Name() string { return "memory" }

func (o *Observer) IngestMetrics(_ context.Context, samples []contract.Sample) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now().UnixMicro()
	for _, s := range samples {
		if s.TimeUS == 0 {
			s.TimeUS = now
		}
		o.samples = append(o.samples, s)
	}
	return nil
}

func (o *Observer) QueryMetrics(_ context.Context, q contract.MetricQuery) ([]contract.Series, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	by := map[string]*contract.Series{}
	for _, s := range o.samples {
		if q.Name != "" && s.Name != q.Name {
			continue
		}
		if q.StartUS != 0 && s.TimeUS < q.StartUS {
			continue
		}
		if q.EndUS != 0 && s.TimeUS > q.EndUS {
			continue
		}
		ser, ok := by[s.Name]
		if !ok {
			ser = &contract.Series{Name: s.Name, Labels: s.Labels}
			by[s.Name] = ser
		}
		ser.Points = append(ser.Points, [2]float64{float64(s.TimeUS) / 1e6, s.Value})
	}
	out := make([]contract.Series, 0, len(by))
	for _, s := range by {
		out = append(out, *s)
	}
	return out, nil
}

func (o *Observer) IngestLogs(_ context.Context, recs []contract.Log) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now().UnixMicro()
	for _, r := range recs {
		if r.TimeUS == 0 {
			r.TimeUS = now
		}
		o.logs = append(o.logs, r)
	}
	return nil
}

func (o *Observer) SearchLogs(_ context.Context, q contract.LogQuery) ([]contract.Log, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []contract.Log
	for _, r := range o.logs {
		if q.Stream != "" && r.Stream != q.Stream {
			continue
		}
		if q.Text != "" && !strings.Contains(strings.ToLower(r.Message), strings.ToLower(q.Text)) {
			continue
		}
		if q.StartUS != 0 && r.TimeUS < q.StartUS {
			continue
		}
		if q.EndUS != 0 && r.TimeUS > q.EndUS {
			continue
		}
		out = append(out, r)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}
