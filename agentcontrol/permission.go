package agentcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"time"
)

// permission implements the SDK control_request/can_use_tool exchange. Only an
// explicit structured client decision can grant permission; unknown requests fail.
func (m *Manager) permission(ctx context.Context, p *process, frame []byte) {
	var req struct {
		ID      string          `json:"request_id"`
		Request json.RawMessage `json:"request"`
	}
	if json.Unmarshal(frame, &req) != nil || req.ID == "" {
		return
	}
	var shape struct {
		Subtype string `json:"subtype"`
	}
	_ = json.Unmarshal(req.Request, &shape)
	var answer json.RawMessage
	err := errors.New("unsupported request or missing explicit approval")
	if shape.Subtype == "can_use_tool" && p.state.Request.Observer != nil && m.authorize(ctx, p.state.Request.Actor, p.state.ID, "approve") == nil {
		answer, err = p.state.Request.Observer.Request(ctx, contract.AgentSessionRequest{ID: req.ID, Method: "can_use_tool", Params: req.Request})
		var decision struct {
			Behavior     string         `json:"behavior"`
			UpdatedInput map[string]any `json:"updatedInput"`
			Message      string         `json:"message"`
		}
		if err == nil {
			if json.Unmarshal(answer, &decision) != nil || (decision.Behavior != "allow" && decision.Behavior != "deny") || decision.Behavior == "allow" && decision.UpdatedInput == nil {
				err = errors.New("invalid explicit permission response")
			} else if decision.Behavior == "allow" {
				answer, _ = json.Marshal(map[string]any{"behavior": "allow", "updatedInput": decision.UpdatedInput})
			} else {
				answer, _ = json.Marshal(map[string]any{"behavior": "deny", "message": decision.Message})
			}
		}
	}
	response := map[string]any{"subtype": "success", "request_id": req.ID, "response": answer}
	if err != nil {
		response = map[string]any{"subtype": "error", "request_id": req.ID, "error": err.Error()}
	}
	if ctx.Err() != nil {
		return
	}
	_ = m.write(p, map[string]any{"type": "control_response", "response": response})
}

// Register synchronously before reading another frame, so native cancellation
// cannot overtake a permission goroutine that has not started yet.
func (m *Manager) dispatchPermission(p *process, frame []byte) {
	var req struct {
		ID string `json:"request_id"`
	}
	if json.Unmarshal(frame, &req) != nil || req.ID == "" {
		return
	}
	m.mu.Lock()
	if p.permissions == nil {
		p.permissions = map[string]context.CancelFunc{}
	}
	if p.permissions[req.ID] != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Minute)
	p.permissions[req.ID] = cancel
	m.mu.Unlock()
	go func() {
		defer cancel()
		defer func() { m.mu.Lock(); delete(p.permissions, req.ID); m.mu.Unlock() }()
		m.permission(ctx, p, frame)
	}()
}
