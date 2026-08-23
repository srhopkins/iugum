package app

import (
	"context"
	"fmt"
	"os"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/dirs"
	"github.com/srhopkins/iugum/plugin"
	"github.com/srhopkins/iugum/policy"
)

// App is the composition root. Every public action goes through Gate first.
type App struct {
	Actor     string
	Gate      contract.Policy
	Tracker   contract.Tracker
	Wiki      contract.Wiki
	Observer  contract.Observer
	Memory    contract.Memory
	Scheduler contract.Scheduler
	Hooks     contract.Hooks
	Watcher   contract.Watcher
}

func New(cfg config.File) (*App, error) {
	gate, err := policy.New(cfg.Policy.Model, cfg.Policy.Policy)
	if err != nil {
		return nil, err
	}
	tcfg := map[string]string{}
	wcfg := map[string]string{}
	ocfg := map[string]string{}
	if len(cfg.Exec.Tracker) > 0 {
		tcfg["command"] = join(cfg.Exec.Tracker)
	}
	if len(cfg.Exec.Wiki) > 0 {
		wcfg["command"] = join(cfg.Exec.Wiki)
	}
	if len(cfg.Exec.Observe) > 0 {
		ocfg["command"] = join(cfg.Exec.Observe)
	}
	tr, err := plugin.NewTracker(cfg.Tracker, tcfg)
	if err != nil {
		return nil, err
	}
	wi, err := plugin.NewWiki(cfg.Wiki, wcfg)
	if err != nil {
		return nil, err
	}
	data, err := dirs.ResolveData(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	ocfg["data_dir"] = data
	ob, err := plugin.NewObserver(cfg.Observe, ocfg)
	if err != nil {
		return nil, err
	}
	kind := cfg.Embeddings.Kind
	if !cfg.Embeddings.Enabled && kind != "ollama" && kind != "openai" {
		kind = "off"
	}
	memName := cfg.Memory
	if memName == "" {
		memName = "sqlite"
	}
	vec := "false"
	if cfg.Embeddings.Enabled && cfg.Embeddings.Vec {
		vec = "true"
	}
	mem, err := plugin.NewMemory(memName, map[string]string{
		"data_dir":    data,
		"glossary":    cfg.Graph.Glossary,
		"extractor":   cfg.Graph.Extractor,
		"embed_kind":  kind,
		"embed_url":   cfg.Embeddings.URL,
		"embed_model": cfg.Embeddings.Model,
		"vec":         vec,
		"llm_kind":    cfg.Graph.LLM.Kind,
		"llm_url":     cfg.Graph.LLM.URL,
		"llm_model":   cfg.Graph.LLM.Model,
	})
	if err != nil {
		return nil, err
	}
	sch, err := plugin.NewScheduler("cron", nil)
	if err != nil {
		return nil, err
	}
	hk, err := plugin.NewHooks("bus", nil)
	if err != nil {
		return nil, err
	}
	wt, err := plugin.NewWatcher("fsnotify", nil)
	if err != nil {
		return nil, err
	}
	a := &App{
		Actor:     config.Actor(cfg),
		Gate:      gate,
		Tracker:   tr,
		Wiki:      wi,
		Observer:  ob,
		Memory:    mem,
		Scheduler: sch,
		Hooks:     hk,
		Watcher:   wt,
	}
	a.Hooks.Register("mem.ingest", func(ctx context.Context, ev contract.Event) error {
		text := ""
		if ev.Attrs != nil {
			text = ev.Attrs["text"]
		}
		if text == "" && ev.Path != "" {
			raw, err := os.ReadFile(ev.Path)
			if err != nil {
				return err
			}
			text = string(raw)
		}
		_, err := a.Ingest(ctx, "default", text)
		return err
	})
	a.registerExecJobs(cfg.Jobs)
	a.registerHTTPJobs(cfg.Jobs)
	a.bindBeadsMemory()
	return a, nil
}

func (a *App) Check(ctx context.Context, obj, act string) error {
	return a.Gate.Enforce(ctx, contract.Request{Sub: a.Actor, Obj: obj, Act: act})
}

func (a *App) RunTracker(ctx context.Context, args []string) error {
	if err := a.Check(ctx, "tracker", "run"); err != nil {
		return err
	}
	return a.Tracker.Run(ctx, args)
}

func (a *App) ServeWiki(ctx context.Context, opts contract.WikiOpts) error {
	if err := a.Check(ctx, "wiki", "serve"); err != nil {
		return err
	}
	return a.Wiki.Serve(ctx, opts)
}

func (a *App) IngestMetrics(ctx context.Context, samples []contract.Sample) error {
	if err := a.Check(ctx, "observe", "ingest"); err != nil {
		return err
	}
	return a.Observer.IngestMetrics(ctx, samples)
}

func (a *App) QueryMetrics(ctx context.Context, q contract.MetricQuery) ([]contract.Series, error) {
	if err := a.Check(ctx, "observe", "query"); err != nil {
		return nil, err
	}
	return a.Observer.QueryMetrics(ctx, q)
}

func (a *App) Remember(ctx context.Context, rec contract.MemoryRec) error {
	if err := a.Check(ctx, contract.MemoryObj(rec.Type, rec.NS), "write"); err != nil {
		return err
	}
	return a.Memory.Remember(ctx, rec)
}

func (a *App) Recall(ctx context.Context, ns, key string) (contract.MemoryRec, bool, error) {
	if err := a.Check(ctx, contract.MemoryObj("*", ns), "read"); err != nil {
		return contract.MemoryRec{}, false, err
	}
	return a.Memory.Recall(ctx, ns, key)
}

func (a *App) SearchMem(ctx context.Context, q contract.MemoryQuery) ([]contract.MemoryHit, error) {
	nss := q.NS
	if len(nss) == 0 {
		nss = []string{"default"}
	}
	for _, ns := range nss {
		if err := a.Check(ctx, contract.MemoryObj(q.Type, ns), "read"); err != nil {
			return nil, err
		}
	}
	return a.Memory.Search(ctx, q)
}

func (a *App) AttachMem(ctx context.Context, fromNS, toNS, key string) error {
	if err := a.Check(ctx, contract.MemoryObj("*", fromNS), "read"); err != nil {
		return err
	}
	if err := a.Check(ctx, contract.MemoryObj("*", toNS), "attach"); err != nil {
		return err
	}
	return a.Memory.Attach(ctx, fromNS, toNS, key)
}

func (a *App) SliceMem(ctx context.Context, fromNS, toNS, filter string) error {
	if err := a.Check(ctx, contract.MemoryObj("*", fromNS), "read"); err != nil {
		return err
	}
	if err := a.Check(ctx, contract.MemoryObj("*", toNS), "slice"); err != nil {
		return err
	}
	return a.Memory.Slice(ctx, fromNS, toNS, filter)
}

func (a *App) Ingest(ctx context.Context, ns, text string) ([]contract.MemoryEdge, error) {
	if err := a.Check(ctx, contract.MemoryObj("graph", ns), "write"); err != nil {
		return nil, err
	}
	return a.Memory.Ingest(ctx, ns, text)
}

func (a *App) Walk(ctx context.Context, q contract.WalkQuery) ([]contract.WalkHit, error) {
	if err := a.Check(ctx, contract.MemoryObj("graph", q.NS), "read"); err != nil {
		return nil, err
	}
	return a.Memory.Walk(ctx, q)
}

func (a *App) FireHook(ctx context.Context, ev contract.Event) error {
	if err := a.Check(ctx, "hook", "fire"); err != nil {
		return err
	}
	return a.Hooks.Fire(ctx, ev)
}

func (a *App) TriggerJob(ctx context.Context, name string) error {
	if err := a.Check(ctx, "schedule", "run"); err != nil {
		return err
	}
	return a.Scheduler.Trigger(name)
}

func (a *App) ListenHooksHTTP(ctx context.Context, addr string) error {
	if err := a.Check(ctx, "hook", "http"); err != nil {
		return err
	}
	return a.Hooks.ListenHTTP(addr)
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

func DenyMessage(err error) string {
	if d, ok := err.(contract.Denied); ok {
		return d.Error()
	}
	return fmt.Sprint(err)
}
