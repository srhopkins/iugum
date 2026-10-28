package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var ErrStalePrepared = errors.New("agentcore: prepared turn is expired, consumed, or no longer matches")

// PreparedTurn is an opaque, single-use model decision. It has no public wire
// representation and cannot execute tools before RunPrepared is called.
type PreparedTurn struct {
	message     Message
	fingerprint [32]byte
	profile     string
	expires     time.Time
	used        atomic.Bool
}

// Prepare makes the normal first model request with the normal context and tool
// schemas, but returns BEFORE any tool authorization callback or execution.
// Model usage is still accounted; no conversation persistence happens here.
func (r *Runtime) Prepare(ctx context.Context, q RunRequest, tools []Tool) (*PreparedTurn, error) {
	digest, err := preparationFingerprint(r, q, tools)
	if err != nil {
		return nil, err
	}
	result, err := r.run(ctx, q, tools, nil, true)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if len(result.Messages) == 0 {
		return nil, errors.New("agentcore: empty preparation")
	}
	return &PreparedTurn{message: result.Messages[len(result.Messages)-1], fingerprint: digest, profile: result.Profile, expires: r.Now().Add(30 * time.Second)}, nil
}

// RunPrepared is called only after an actual user submission. Every current
// model/tool policy and argument authorization remains in the ordinary loop.
func (r *Runtime) RunPrepared(ctx context.Context, q RunRequest, tools []Tool, p *PreparedTurn) (RunResult, error) {
	if p == nil {
		return RunResult{}, ErrStalePrepared
	}
	return r.run(ctx, q, tools, p, false)
}
func (p *PreparedTurn) valid(r *Runtime, q RunRequest, tools []Tool, profile string) bool {
	if !r.Now().Before(p.expires) || profile != p.profile {
		return false
	}
	d, e := preparationFingerprint(r, q, tools)
	return e == nil && d == p.fingerprint
}
func preparationFingerprint(r *Runtime, q RunRequest, tools []Tool) ([32]byte, error) {
	messages, e := modelMessages(q.Messages, true)
	if e != nil {
		return [32]byte{}, e
	}
	defs := []any{}
	for _, t := range tools {
		defs = append(defs, map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters})
	}
	b, e := json.Marshal(struct {
		Actor, Profile string
		Messages       any
		Config         Config
		Tools          []any
	}{q.Actor, q.Profile, messages, r.config, defs})
	if e != nil {
		return [32]byte{}, e
	}
	return sha256.Sum256(b), nil
}
func validatePreparedMessage(m Message, tools map[string]Tool) error {
	if m.Role != "assistant" || len(m.Images) > 0 {
		return errors.New("agentcore: invalid prepared assistant message")
	}
	seen := map[string]bool{}
	for _, c := range m.ToolCalls {
		if c.ID == "" || seen[c.ID] || c.Type != "function" {
			return errors.New("agentcore: invalid prepared tool call")
		}
		seen[c.ID] = true
		if _, ok := tools[c.Function.Name]; !ok {
			return fmt.Errorf("agentcore: unknown prepared tool %q", c.Function.Name)
		}
		var args map[string]json.RawMessage
		if json.Unmarshal([]byte(c.Function.Arguments), &args) != nil || args == nil {
			return errors.New("agentcore: prepared tool arguments must be an object")
		}
	}
	if len(m.ToolCalls) == 0 && m.Content == "" {
		return errors.New("agentcore: empty prepared reply")
	}
	return nil
}
