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

// Memory is the persistent-insight slot. Default: SQLite file. Tickets stay on Dolt.
// One truth row. Namespaces attach, slice, and join that row. They do not copy it.
// Type is fact now. Later: episode, wiki, graph, code (SCIP / LSP index).
type Memory interface {
	Name() string
	Remember(ctx context.Context, rec MemoryRec) error
	Recall(ctx context.Context, ns, key string) (MemoryRec, bool, error)
	Forget(ctx context.Context, ns, key string) error
	Search(ctx context.Context, q MemoryQuery) ([]MemoryHit, error)
	Attach(ctx context.Context, fromNS, toNS, key string) error
	Detach(ctx context.Context, ns, key string) error
	Slice(ctx context.Context, fromNS, toNS, filter string) error
	Link(ctx context.Context, ns string, e MemoryEdge) error
	Unlink(ctx context.Context, ns, from, rel, to string) error
	Walk(ctx context.Context, q WalkQuery) ([]WalkHit, error)
	Ingest(ctx context.Context, ns, text string) ([]MemoryEdge, error)
}

// MemoryRec is one stored insight.
type MemoryRec struct {
	NS    string // namespace path, default "default"
	Type  string // fact | episode | wiki | graph | code
	Key   string
	Value string
}

// MemoryQuery is a search. NS is a join (union). Empty NS means "default".
// Text is always used. Vector search runs when embeddings are on.
type MemoryQuery struct {
	NS    []string
	Type  string
	Text  string
	Limit int
}

// MemoryHit is one search result.
type MemoryHit struct {
	NS    string
	Type  string
	Key   string
	Value string
	Score float64
}

// MemoryEdge is one graph link. From and To are memory keys in the namespace.
type MemoryEdge struct {
	From  string
	Rel   string
	To    string
	Value string
}

// WalkQuery follows edges. Rel empty means any relation. Hops default 2.
type WalkQuery struct {
	NS   string
	From string
	Rel  string
	Hops int
}

// WalkHit is one hop in a walk.
type WalkHit struct {
	From string
	Rel  string
	To   string
	Hops int
}

// MemoryObj is the Casbin object: mem/{type}/ns/{path}
func MemoryObj(typ, ns string) string {
	if typ == "" {
		typ = "fact"
	}
	if ns == "" {
		ns = "default"
	}
	return "mem/" + typ + "/ns/" + ns
}

// Embedder turns text into a vector. Off means Search is substring only.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// Observer is the metrics+logs slot. Default: in-process memory until sqlite lands.
// Agents query metrics with PromQL and logs with LogQL (see NORTHSTARS.md).
// MetricQuery / LogQuery grow an Expr field when those parsers land.
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

// Event is one hook payload. Local Fire and later HTTP POST /hooks/{name} use this.
// HTTP body later: {event, delivery_id, ts, data}. HMAC-SHA256 over the raw body.
type Event struct {
	Name   string            // hook name: watch.changed, cron.tick, http.ingest
	Source string            // watch | cron | hook | http
	Path   string
	Attrs  map[string]string
}

// JobFunc is one job body. A job is not a plugin. A plugin is a compiled adapter.
type JobFunc func(ctx context.Context, ev Event) error

// Scheduler is cron + adhoc trigger. Default: netresearch/go-cron.
type Scheduler interface {
	Name() string
	Add(spec, jobName string, fn JobFunc) error
	// AddAfter wires child to run when parent finishes with the given outcome.
	// on is success, failure, skipped, or complete (maps to go-cron TriggerCondition).
	AddAfter(childName, parentName, on string) error
	Trigger(jobName string) error
	Start() error
	Stop() error
}

// Watcher subscribes to OS file events. Default: fsnotify.
type Watcher interface {
	Name() string
	Add(path string) error
	Events() <-chan Event
	Close() error
}

// Hooks routes named events to jobs. HTTP listen is reserved (stub).
type Hooks interface {
	Register(jobName string, fn JobFunc)
	AddWorkflow(name string, steps []string)
	On(hookName, jobName string)
	Fire(ctx context.Context, ev Event) error
	ListenHTTP(addr string) error
}

// HTTPHookStub is the reserved webhook error. Do not bind a port yet.
type HTTPHookStub struct {
	Path string // POST /hooks/{name}
}

func (e HTTPHookStub) Error() string {
	return "iugum: hook HTTP listen is a stub; reserved path " + e.Path
}
