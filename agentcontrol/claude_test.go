package agentcontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testGate struct{ deny string }

func (g testGate) Enforce(_ context.Context, r contract.Request) error {
	if r.Act == g.deny {
		return contract.Denied{Req: r}
	}
	return nil
}
func TestClaudeProcessHelper(t *testing.T) {
	isHelper := false
	for _, arg := range os.Args {
		if arg == "--print" {
			isHelper = true
		}
	}
	if !isHelper {
		return
	}
	var mu sync.Mutex
	emit := func(v any) { mu.Lock(); defer mu.Unlock(); _ = json.NewEncoder(os.Stdout).Encode(v) }
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var in map[string]json.RawMessage
		_ = json.Unmarshal(scanner.Bytes(), &in)
		var typ string
		_ = json.Unmarshal(in["type"], &typ)
		if typ == "control_request" {
			var id string
			_ = json.Unmarshal(in["request_id"], &id)
			emit(map[string]any{"type": "control_response", "response": map[string]any{"request_id": id, "subtype": "success", "response": map[string]any{}}})
			continue
		}
		emit(map[string]any{"type": "system", "subtype": "init", "session_id": "saved-session"})
		go func() {
			time.Sleep(150 * time.Millisecond)
			emit(map[string]any{"type": "result", "session_id": "saved-session", "result": "done", "usage": map[string]any{"input_tokens": 123, "output_tokens": 7, "cache_read_input_tokens": 40}, "total_cost_usd": 0.25})
		}()
	}
	os.Exit(0)
}
func setup(t *testing.T) (*Manager, StartRequest) {
	t.Helper()
	dir := t.TempDir()
	binary, _ := os.Executable()
	script := filepath.Join(dir, "claude")
	err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\nexec %q -test.run=TestClaudeProcessHelper -- \"$@\"\n", binary)), 0700)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(dir, "state"), testGate{})
	if err != nil {
		t.Fatal(err)
	}
	m.Executable = script
	m.VerifyAccount = func(context.Context, string, StartRequest) error { return nil }
	return m, StartRequest{Actor: "chief", ID: "managed", Project: dir, Account: "personal", ConfigDir: dir, MaxDuration: 5 * time.Second}
}
func TestManagedStreamQueueReconnect(t *testing.T) {
	m, r := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := m.Start(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "idle" {
		t.Fatal(s)
	}
	s, err = m.Send(ctx, "chief", r.ID, "first")
	if err != nil || s.Status != "running" {
		t.Fatal(s, err)
	}
	s, err = m.Send(ctx, "chief", r.ID, "second")
	if err != nil || len(s.Pending) != 1 {
		t.Fatal(s, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, _ = m.Snapshot(ctx, "chief", r.ID)
		if s.Status == "idle" && len(s.Pending) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.Status != "idle" || s.SessionID != "saved-session" {
		t.Fatal(s)
	}
	if !s.Usage.TokensKnown || s.Usage.InputTokens == nil || *s.Usage.InputTokens != 123 || s.Usage.BilledKnown || !s.Usage.EstimateKnown {
		t.Fatal(s.Usage)
	}
	events, err := m.Events(ctx, "chief", r.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	inputs := 0
	for _, ev := range events {
		if ev.Direction == "input" && strings.Contains(string(ev.Data), "Current timestamp:") {
			inputs++
		}
	}
	if inputs != 2 {
		t.Fatal(inputs, events)
	}
	if err = m.Interrupt(ctx, "chief", r.ID); err != nil {
		t.Fatal(err)
	}
	if err = m.Stop(ctx, "chief", r.ID); err != nil {
		t.Fatal(err)
	}
	s, err = m.Reconnect(ctx, "chief", r.ID)
	if err != nil || s.Request.ResumeSessionID != "saved-session" {
		t.Fatal(s, err)
	}
	_ = m.Stop(ctx, "chief", r.ID)
}
func TestPolicyDenialAndUnmanaged(t *testing.T) {
	m, r := setup(t)
	m.gate = testGate{deny: "start"}
	_, err := m.Start(context.Background(), r)
	if err == nil {
		t.Fatal("start allowed")
	}
	m.gate = testGate{}
	_, err = m.Send(context.Background(), "chief", "unknown", "go")
	if !errors.Is(err, ErrUnmanaged) {
		t.Fatal(err)
	}
}
