package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/agentdesk"
	"github.com/srhopkins/iugum/contract"
	"path/filepath"
	"testing"
)

type controlTestGate struct{}

func (controlTestGate) Enforce(context.Context, contract.Request) error { return nil }
func TestControlApprovalPreparation(t *testing.T) {
	dir := t.TempDir()
	manager, err := agentcontrol.New(filepath.Join(dir, "state"), controlTestGate{})
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	var requested agentdesk.ApprovalRequest
	bridge := newAgentControlBridge(context.Background(), manager, "chief", map[string]string{"chief@example.org": dir}, func(context.Context, string, string) error { return nil }, func(_ context.Context, r agentdesk.ApprovalRequest) (agentdesk.Approval, error) {
		requests++
		requested = r
		return agentdesk.Approval{ID: "one", ApprovalRequest: r}, nil
	})
	var execute func(context.Context, json.RawMessage) (string, error)
	for _, tool := range bridge.Tools() {
		if tool.Name == "claude_request_start" {
			execute = tool.Execute
		}
	}
	raw, _ := json.Marshal(map[string]any{"project": dir, "account": "chief@example.org", "prompt": "Read the test fixture and report findings.", "allowed_tools": []string{"Read"}})
	if _, err = execute(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || requested.Action != "claude_managed_start" {
		t.Fatal(requested)
	}
	var payload agentControlStartPayload
	_ = json.Unmarshal(requested.Payload, &payload)
	if payload.ConfigDir != dir || payload.MaxDurationMinutes != 480 {
		t.Fatal(payload)
	}
	states, err := manager.List(context.Background(), "chief")
	if err != nil || len(states) != 0 {
		t.Fatalf("approval preparation launched work: %v %v", states, err)
	}
}
func TestControlRejectsAccountFallbackAndRevokedScope(t *testing.T) {
	dir := t.TempDir()
	m, _ := agentcontrol.New(filepath.Join(dir, "state"), controlTestGate{})
	b := newAgentControlBridge(context.Background(), m, "chief", map[string]string{"chief@example.org": dir}, func(context.Context, string, string) error { return errors.New("denied") }, nil)
	p := agentControlStartPayload{Project: dir, Account: "contract@example.org", Prompt: "do work"}
	if err := b.validateStart(context.Background(), &p); err == nil {
		t.Fatal("unconfigured account accepted")
	}
	p.Account = "chief@example.org"
	if err := b.validateStart(context.Background(), &p); err == nil {
		t.Fatal("revoked policy ignored")
	}
}
