// Package contract is the public adapter API.
// Third-party modules implement these interfaces and register in plugin.
package contract

import "context"

// Request is what Casbin (the policy engine) sees on every gated call.
// Sub is who (actor). Obj is what (tracker, wiki, observe, …). Act is the verb (run, serve, ingest).
type Request struct {
	Sub string
	Obj string
	Act string
}

// Policy is the gate. Every command and adapter call goes through Enforce first.
type Policy interface {
	Enforce(ctx context.Context, req Request) error
}

// Tracker is the work-graph slot. Default: beads (same CLI as bd).
type Tracker interface {
	Name() string
	Run(ctx context.Context, args []string) error
}

// Wiki is the notes-server slot. Default: embedded SilverBullet.
type Wiki interface {
	Name() string
	Serve(ctx context.Context, opts WikiOpts) error
}

// WikiOpts is how iugum starts a wiki adapter.
type WikiOpts struct {
	Port  int
	Host  string
	Space string
}

// Observer is the metrics+logs slot. Default: in-process memory until sqlite lands.
type Observer interface {
	Name() string
	IngestMetrics(ctx context.Context, samples []Sample) error
	QueryMetrics(ctx context.Context, q MetricQuery) ([]Series, error)
	IngestLogs(ctx context.Context, recs []Log) error
	SearchLogs(ctx context.Context, q LogQuery) ([]Log, error)
}

// Sample is one metric point in degrees or percents as the adapter documents.
// iugum graphs temperatures in °C, never millidegrees.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
	TimeUS int64 // unix microseconds; 0 = now
}

// MetricQuery is a time range + name filter.
type MetricQuery struct {
	Name     string
	StartUS  int64
	EndUS    int64
	MaxPoints int
}

// Series is one named line.
type Series struct {
	Name   string
	Labels map[string]string
	Points [][2]float64 // [unix_seconds, value]
}

// Log is one log line.
type Log struct {
	TimeUS  int64
	Stream  string
	Level   string
	Message string
	Attrs   map[string]string
}

// LogQuery is a text + stream filter.
type LogQuery struct {
	Stream  string
	Text    string
	StartUS int64
	EndUS   int64
	Limit   int
}

// Denied is a policy refusal.
type Denied struct {
	Req Request
}

func (d Denied) Error() string {
	return "iugum: policy denied " + d.Req.Sub + " " + d.Req.Act + " " + d.Req.Obj
}
