package hookbus

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/srhopkins/iugum/adapter/hook/httpserve"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterHooks("bus", func(map[string]string) (contract.Hooks, error) {
		return New(), nil
	})
}

// Bus routes hook names to jobs.
type Bus struct {
	mu     sync.Mutex
	jobs   map[string]contract.JobFunc
	routes map[string][]string
}

func New() *Bus {
	return &Bus{
		jobs:   map[string]contract.JobFunc{},
		routes: map[string][]string{},
	}
}

func (b *Bus) Register(jobName string, fn contract.JobFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.jobs[jobName] = fn
}

func (b *Bus) On(hookName, jobName string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.routes[hookName] = append(b.routes[hookName], jobName)
}

func (b *Bus) Fire(ctx context.Context, ev contract.Event) error {
	b.mu.Lock()
	jobs := append([]string{}, b.routes[ev.Name]...)
	b.mu.Unlock()
	if ev.Name != "" {
		if fn := b.job(ev.Name); fn != nil && len(jobs) == 0 {
			return fn(ctx, ev)
		}
	}
	for _, name := range jobs {
		fn := b.job(name)
		if fn == nil {
			continue
		}
		if err := fn(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bus) job(name string) contract.JobFunc {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.jobs[name]
}

func (b *Bus) ListenHTTP(addr string) error {
	if addr == "" {
		return contract.HTTPHookStub{Path: "/hooks/{name}"}
	}
	secret := os.Getenv("IUGUM_HOOK_SECRET")
	return httpserve.Start(addr, secret, b.Fire)
}

// AddWorkflow registers a linear pipeline. Not a DAG. Fail stops the rest.
func (b *Bus) AddWorkflow(name string, steps []string) {
	b.Register(name, func(ctx context.Context, ev contract.Event) error {
		for _, step := range steps {
			fn := b.job(step)
			if fn == nil {
				return fmt.Errorf("iugum: unknown job %q in workflow %q", step, name)
			}
			if err := fn(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	})
}
