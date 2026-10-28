package beadactivity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// bd --set-metadata stores object text as a JSON string in show --json.
func cliMetadata(t *testing.T, value any) json.RawMessage {
	t.Helper()
	object, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(string(object))
	if err != nil {
		t.Fatal(err)
	}
	return quoted
}

func TestCanonicalCLIStringActivityProjection(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 35, 59, 0, time.UTC)
	signal := Signal{RunID: "real-run", Actor: "worker", State: "working", SeenAt: now, ExpiresAt: now.Add(time.Minute)}
	raw := cliMetadata(t, signal)
	for _, tc := range []struct {
		name, owner string
		at          time.Time
		want        string
	}{{"fresh", "worker", now, "working"}, {"expired", "worker", now.Add(time.Minute), "stale"}, {"wrong owner", "other", now, "stale"}} {
		t.Run(tc.name, func(t *testing.T) {
			got := Project(tc.owner, "in_progress", Decode(map[string]json.RawMessage{MetadataKey: raw}), tc.at)
			if got.State != tc.want || got.RunID != "real-run" {
				t.Fatalf("got %+v, want %s with real run", got, tc.want)
			}
		})
	}
}

func TestCanonicalCLIStringExecutionHistoryAndGuards(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 35, 59, 0, time.UTC)
	started := now.Add(-time.Hour)
	prior := Execution{Version: 1, RunID: "real-run", Actor: "worker", Label: "worker", Platform: "codex", Host: "mac", SessionID: "session", ContextSessionID: "context", ParentSessionID: "parent", ParentAgentID: "parent-agent", Transcript: "trace", ThreadURL: "https://example.test/thread", CWD: "/workspace", StartedAt: started, LastSeen: now}
	other := prior
	other.RunID = "older-run"
	refs := Executions(map[string]json.RawMessage{executionKey(prior.RunID): cliMetadata(t, prior), executionKey(other.RunID): cliMetadata(t, other)})
	if len(refs) != 2 {
		t.Errorf("lost canonical execution history: %+v", refs)
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*Execution)
		ended   bool
		wantErr bool
	}{
		{name: "preserve original start and references"},
		{name: "session drift", mutate: func(e *Execution) { e.SessionID = "changed" }, wantErr: true},
		{name: "host drift", mutate: func(e *Execution) { e.Host = "changed" }, wantErr: true},
		{name: "context drift", mutate: func(e *Execution) { e.ContextSessionID = "changed" }, wantErr: true},
		{name: "parent drift", mutate: func(e *Execution) { e.ParentSessionID = "changed" }, wantErr: true},
		{name: "completed run cannot restart", ended: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := prior
			if tc.ended {
				stored.EndedAt = &now
			}
			show, _ := json.Marshal([]any{map[string]any{"id": "ticket", "assignee": "worker", "status": "in_progress", "metadata": map[string]json.RawMessage{executionKey(prior.RunID): cliMetadata(t, stored)}}})
			incoming := prior
			incoming.StartedAt = time.Time{}
			incoming.ContextSessionID = ""
			incoming.ParentSessionID = ""
			incoming.ParentAgentID = ""
			incoming.Transcript = ""
			incoming.ThreadURL = ""
			if tc.mutate != nil {
				tc.mutate(&incoming)
			}
			writes := 0
			runner := func(args ...string) ([]byte, error) {
				if args[0] == "show" {
					return show, nil
				}
				writes++
				for _, a := range args {
					if strings.HasPrefix(a, executionKey(prior.RunID)+"=") {
						var saved Execution
						if err := json.Unmarshal([]byte(strings.SplitN(a, "=", 2)[1]), &saved); err != nil {
							t.Fatal(err)
						}
						if !saved.StartedAt.Equal(started) || saved.ContextSessionID != "context" || saved.ParentSessionID != "parent" || saved.ParentAgentID != "parent-agent" || saved.Transcript != "trace" || saved.ThreadURL != "https://example.test/thread" {
							t.Errorf("lost execution history %+v", saved)
						}
					}
				}
				return nil, nil
			}
			err := ReportExecution(runner, incoming, "ticket", "working", "", now, time.Minute)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && writes != 0 {
				t.Fatal("invalid reuse wrote metadata")
			}
			if !tc.wantErr && writes != 1 {
				t.Fatalf("writes %d", writes)
			}
		})
	}
}

func TestMetadataRejectsNestedStringsAndInvalidValues(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`1`), json.RawMessage(`"garbage"`), json.RawMessage(`"{bad}"`), cliMetadata(t, `{"actor":"worker"}`)} {
		if got := Decode(map[string]json.RawMessage{MetadataKey: raw}); got != nil {
			t.Errorf("decoded invalid %s as %+v", raw, got)
		}
		if got := Executions(map[string]json.RawMessage{executionKey("run"): raw}); len(got) != 0 {
			t.Errorf("decoded invalid execution %s", raw)
		}
	}
}
