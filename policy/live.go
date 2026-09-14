package policy

import (
	"context"
	"github.com/srhopkins/iugum/contract"
)

// Live reloads an administrator-owned policy on each call. A missing or invalid
// file is an error, never a fallback to permissive policy.
type Live struct {
	Path      string
	ModelPath string
}

func (l Live) Enforce(ctx context.Context, r contract.Request) error {
	g, e := New(l.ModelPath, l.Path)
	if e != nil {
		return e
	}
	return g.Enforce(ctx, r)
}

// OpenRun is an explicit runtime override. It has no save/marshal mechanism.
// Host code decides whether to construct it; plugin configuration cannot.
type OpenRun struct{}

func (OpenRun) Enforce(context.Context, contract.Request) error { return nil }
