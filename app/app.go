package app

import (
	"context"
	"fmt"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
	"github.com/srhopkins/iugum/policy"
)

// App is the composition root. Every public action goes through Gate first.
type App struct {
	Actor    string
	Gate     contract.Policy
	Tracker  contract.Tracker
	Wiki     contract.Wiki
	Observer contract.Observer
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
	ob, err := plugin.NewObserver(cfg.Observe, ocfg)
	if err != nil {
		return nil, err
	}
	return &App{
		Actor:    config.Actor(cfg),
		Gate:     gate,
		Tracker:  tr,
		Wiki:     wi,
		Observer: ob,
	}, nil
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
