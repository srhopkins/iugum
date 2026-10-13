package app

import (
	"context"
	"sync"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

// lazyMemory opens the memory store on first use. Commands that never touch
// memory (for example every ticket command in `iugum beads`) never open the
// SQLite file, so they cannot contend for its lock.
type lazyMemory struct {
	name string
	cfg  map[string]string
	once sync.Once
	mem  contract.Memory
	err  error
}

func newLazyMemory(name string, cfg map[string]string) (*lazyMemory, error) {
	if err := plugin.CheckMemory(name); err != nil {
		return nil, err
	}
	return &lazyMemory{name: name, cfg: cfg}, nil
}

func (l *lazyMemory) open() (contract.Memory, error) {
	l.once.Do(func() { l.mem, l.err = plugin.NewMemory(l.name, l.cfg) })
	return l.mem, l.err
}

func (l *lazyMemory) Name() string { return l.name }

func (l *lazyMemory) Remember(ctx context.Context, rec contract.MemoryRec) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Remember(ctx, rec)
}

func (l *lazyMemory) Recall(ctx context.Context, ns, key string) (contract.MemoryRec, bool, error) {
	m, err := l.open()
	if err != nil {
		return contract.MemoryRec{}, false, err
	}
	return m.Recall(ctx, ns, key)
}

func (l *lazyMemory) Forget(ctx context.Context, ns, key string) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Forget(ctx, ns, key)
}

func (l *lazyMemory) Search(ctx context.Context, q contract.MemoryQuery) ([]contract.MemoryHit, error) {
	m, err := l.open()
	if err != nil {
		return nil, err
	}
	return m.Search(ctx, q)
}

func (l *lazyMemory) Attach(ctx context.Context, fromNS, toNS, key string) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Attach(ctx, fromNS, toNS, key)
}

func (l *lazyMemory) Detach(ctx context.Context, ns, key string) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Detach(ctx, ns, key)
}

func (l *lazyMemory) Slice(ctx context.Context, fromNS, toNS, filter string) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Slice(ctx, fromNS, toNS, filter)
}

func (l *lazyMemory) Link(ctx context.Context, ns string, e contract.MemoryEdge) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Link(ctx, ns, e)
}

func (l *lazyMemory) Unlink(ctx context.Context, ns, from, rel, to string) error {
	m, err := l.open()
	if err != nil {
		return err
	}
	return m.Unlink(ctx, ns, from, rel, to)
}

func (l *lazyMemory) Walk(ctx context.Context, q contract.WalkQuery) ([]contract.WalkHit, error) {
	m, err := l.open()
	if err != nil {
		return nil, err
	}
	return m.Walk(ctx, q)
}

func (l *lazyMemory) Ingest(ctx context.Context, ns, text string) ([]contract.MemoryEdge, error) {
	m, err := l.open()
	if err != nil {
		return nil, err
	}
	return m.Ingest(ctx, ns, text)
}
