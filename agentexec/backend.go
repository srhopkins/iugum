// Package agentexec reserves the policy-mediated process execution boundary.
// A backend must explicitly implement isolation; this contract provides none.
package agentexec

import (
	"context"
	"errors"
)

var ErrUnsupported = errors.New("requested execution isolation is not implemented")

type Request struct {
	Namespace string
	Command   []string
	Directory string
}
type Result struct {
	ExitCode int
	Output   string
}
type Backend interface {
	Execute(context.Context, Request) (Result, error)
}
type Unsupported struct{}

func (Unsupported) Execute(context.Context, Request) (Result, error) { return Result{}, ErrUnsupported }
