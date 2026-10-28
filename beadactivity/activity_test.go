package beadactivity

import (
	"testing"
	"time"
)

func TestProjectionDoesNotInferLivenessFromAssignmentOrStatus(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	fresh := Signal{Actor: "worker", State: "working", SeenAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute)}
	for _, tc := range []struct {
		name, owner, status string
		signal              *Signal
		want                string
	}{
		{"fresh", "worker", "in_progress", &fresh, "working"},
		{"assignment only", "worker", "in_progress", nil, "unknown"},
		{"unassigned", "", "in_progress", &fresh, "unassigned"},
		{"wrong owner", "other", "in_progress", &fresh, "stale"},
		{"closed wins", "worker", "closed", &fresh, "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Project(tc.owner, tc.status, tc.signal, now); got.State != tc.want {
				t.Fatalf("got %s want %s", got.State, tc.want)
			}
		})
	}
	for _, state := range []string{"paused", "completed", "working"} {
		s := fresh
		s.State = state
		s.ExpiresAt = now
		want := state
		if state == "working" {
			want = "stale"
		}
		if got := Project("worker", "in_progress", &s, now); got.State != want {
			t.Errorf("expired %s got %s", state, got.State)
		}
	}
}

func TestSignalRequiresCurrentOwnerAndBoundedFreshness(t *testing.T) {
	now := time.Now().UTC()
	if _, err := NewSignal("worker", "other", "working", "", now, time.Minute); err == nil {
		t.Fatal("accepted nonowner")
	}
	if _, err := NewSignal("", "", "working", "", now, time.Minute); err == nil {
		t.Fatal("accepted unclaimed")
	}
	for _, ttl := range []time.Duration{0, -time.Second, 11 * time.Minute} {
		if _, err := NewSignal("worker", "worker", "working", "", now, ttl); err == nil {
			t.Fatalf("accepted ttl %v", ttl)
		}
	}
	s, err := NewSignal("worker", "worker", "paused", "waiting for review", now, time.Minute)
	if err != nil || s.Reason != "waiting for review" {
		t.Fatalf("signal %v %v", s, err)
	}
	if _, err := NewSignal("worker", "worker", "invented", "", now, time.Minute); err == nil {
		t.Fatal("accepted invalid state")
	}
}

func TestMalformedAndFutureSignalsNeverSpin(t *testing.T) {
	now := time.Now().UTC()
	for _, s := range []Signal{
		{Actor: "worker", State: "working", SeenAt: now.Add(time.Hour), ExpiresAt: now.Add(2 * time.Hour)},
		{Actor: "worker", State: "working", SeenAt: now, ExpiresAt: now.Add(time.Hour)},
		{Actor: "worker", State: "working", ExpiresAt: now.Add(time.Minute)},
	} {
		if got := Project("worker", "in_progress", &s, now); got.State == "working" {
			t.Fatalf("malformed signal spins: %+v", s)
		}
	}
}

func TestReportWritesMetadataOnlyAfterOwnerCheck(t *testing.T) {
	now := time.Now().UTC()
	calls := 0
	run := func(args ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte(`[{"id":"ticket","assignee":"worker","status":"in_progress"}]`), nil
		}
		if len(args) < 5 || args[0] != "--actor" || args[1] != "worker" || args[2] != "update" || args[3] != "ticket" || args[4] != "--set-metadata" {
			t.Fatalf("unexpected mutation %v", args)
		}
		return nil, nil
	}
	if err := Report(run, "worker", "ticket", "working", "", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
	calls = 0
	if err := Report(run, "other", "ticket", "working", "", now, time.Minute); err == nil {
		t.Fatal("nonowner reported")
	}
	if calls != 1 {
		t.Fatal("mutated before owner validation")
	}
}
