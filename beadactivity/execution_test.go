package beadactivity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExecutionsKeepPreviousRunsAndRequireExplicitIdentity(t *testing.T) {
	now := time.Now().UTC()
	e := Execution{RunID: "run-2", Actor: "worker", Label: "Backend worker", Platform: "codex", Host: "mac", SessionID: "actual-thread", ParentSessionID: "actual-parent", CWD: "/workspace", StartedAt: now}
	if err := e.Validate("worker"); err != nil {
		t.Fatal(err)
	}
	e.Actor = "other"
	if e.Validate("worker") == nil {
		t.Fatal("accepted identity mismatch")
	}
	e.Actor = "worker"
	e.SessionID = ""
	if e.Validate("worker") == nil {
		t.Fatal("accepted missing execution identity")
	}
	e.SessionID = "actual-thread"
	calls := 0
	run := func(args ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte(`[{"id":"ticket","assignee":"worker","status":"in_progress","metadata":{"iugum.execution.run-1":{"run_id":"run-1"}}}]`), nil
		}
		for i, a := range args {
			if a == "--set-metadata" && i+1 < len(args) {
				if strings.HasPrefix(args[i+1], executionKey("run-2")+"=") {
					return nil, nil
				}
			}
		}
		t.Fatalf("missing independent execution record %v", args)
		return nil, nil
	}
	if err := ReportExecution(run, e, "ticket", "working", "", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if string(raw) == "" {
		t.Fatal("empty ref")
	}
}

func TestRunReusePreservesStartAndRejectsIdentityDrift(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-time.Hour)
	e := Execution{Version: 1, RunID: "real-run", Actor: "worker", Label: "worker", Platform: "codex", Host: "mac", SessionID: "session", CWD: "/workspace", StartedAt: started}
	prior, _ := json.Marshal(e)
	metadata := map[string]json.RawMessage{executionKey(e.RunID): prior}
	show, _ := json.Marshal([]any{map[string]any{"id": "ticket", "assignee": "worker", "status": "in_progress", "metadata": metadata}})
	writes := 0
	run := func(args ...string) ([]byte, error) {
		if args[0] == "show" {
			return show, nil
		}
		writes++
		for _, arg := range args {
			if strings.HasPrefix(arg, executionKey(e.RunID)+"=") {
				var saved Execution
				if err := json.Unmarshal([]byte(strings.SplitN(arg, "=", 2)[1]), &saved); err != nil {
					t.Fatal(err)
				}
				if !saved.StartedAt.Equal(started) {
					t.Fatal("lost original start")
				}
			}
		}
		return nil, nil
	}
	e.StartedAt = time.Time{}
	if err := ReportExecution(run, e, "ticket", "paused", "review", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	e.SessionID = "different"
	if err := ReportExecution(run, e, "ticket", "working", "", now, time.Minute); err == nil {
		t.Fatal("reused run for different session")
	}
	if writes != 1 {
		t.Fatal("identity drift wrote metadata")
	}
}

func TestExecutionHistoryExtractionKeepsDistinctParentAndAgentReferences(t *testing.T) {
	now := time.Now().UTC()
	a := Execution{Version: 1, RunID: "first", Actor: "worker", Label: "worker", Platform: "codex", Host: "mac", SessionID: "session", AgentID: "agent", ParentSessionID: "parent-session", ParentAgentID: "parent-agent", CWD: "/workspace", StartedAt: now, LastSeen: now}
	b := a
	b.RunID = "second"
	b.StartedAt = now.Add(time.Minute)
	ar, _ := json.Marshal(a)
	br, _ := json.Marshal(b)
	refs := Executions(map[string]json.RawMessage{executionKey(a.RunID): ar, executionKey(b.RunID): br, "private": json.RawMessage(`"never exposed"`)})
	if len(refs) != 2 {
		t.Fatal("lost execution history", refs)
	}
	for _, r := range refs {
		if r.SessionID == r.AgentID || r.ParentSessionID == r.ParentAgentID || r.ParentSessionID != "parent-session" {
			t.Fatal("conflated execution identity", r)
		}
	}
}
