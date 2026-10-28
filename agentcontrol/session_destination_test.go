package agentcontrol

import (
	"context"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingSessionSentinelAfterAuthorization(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, testGate{})
	if err != nil {
		t.Fatal(err)
	}
	for _, denial := range []string{"send", "read"} {
		manager.gate = testGate{deny: denial}
		_, err = manager.SnapshotForSend(context.Background(), "fixture", "PRIVATE_ID")
		if err == nil || errors.Is(err, ErrSessionNotFound) {
			t.Fatal("destination existence exposed before policy")
		}
	}
	manager.gate = testGate{}
	_, err = manager.SnapshotForSend(context.Background(), "fixture", "PRIVATE_ID")
	if !errors.Is(err, ErrSessionNotFound) || strings.Contains(err.Error(), "PRIVATE_ID") || strings.Contains(err.Error(), root) {
		t.Fatal("missing state lacks safe sentinel")
	}
	_, err = manager.SnapshotForSend(context.Background(), "fixture", "../PRIVATE_ID")
	if err == nil || errors.Is(err, ErrSessionNotFound) {
		t.Fatal("invalid ID became missing state")
	}
	if err = os.WriteFile(filepath.Join(root, "corrupt.json"), []byte("PRIVATE_CORRUPT"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = manager.SnapshotForSend(context.Background(), "fixture", "corrupt")
	if err == nil || errors.Is(err, ErrSessionNotFound) {
		t.Fatal("corrupt state became missing state")
	}
	if err = os.Mkdir(filepath.Join(root, "unreadable.json"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = manager.SnapshotForSend(context.Background(), "fixture", "unreadable")
	if err == nil || errors.Is(err, ErrSessionNotFound) {
		t.Fatal("other filesystem error became missing state")
	}
}

type destinationSentinelPolicy struct{ err error }

func (g destinationSentinelPolicy) Enforce(context.Context, contract.Request) error { return g.err }

func TestPolicyCauseCannotForgeDestinationProvenance(t *testing.T) {
	for _, reason := range []error{ErrSessionNotFound, ErrUnmanaged} {
		for _, cause := range []error{fmt.Errorf("PRIVATE_DENIAL: %w", reason), &prequeueDestinationError{reason: reason}} {
			manager, err := New(t.TempDir(), destinationSentinelPolicy{err: cause})
			if err != nil {
				t.Fatal(err)
			}
			for _, send := range []bool{false, true} {
				if send {
					_, err = manager.Send(context.Background(), "fixture", "missing", "prompt")
				} else {
					_, err = manager.SnapshotForSend(context.Background(), "fixture", "missing")
				}
				if err == nil || IsPrequeueDestinationError(err, reason) || !errors.Is(err, reason) {
					t.Fatal("policy cause forged destination provenance or chain lost")
				}
			}
		}
	}
	wrapped := fmt.Errorf("PRIVATE_DENIAL: %w", &prequeueDestinationError{reason: ErrUnmanaged})
	if IsPrequeueDestinationError(wrapped, ErrUnmanaged) {
		t.Fatal("predicate traversed an arbitrary cause")
	}
}

type destinationPolicyFunc func(context.Context, contract.Request) error

func (f destinationPolicyFunc) Enforce(ctx context.Context, r contract.Request) error {
	return f(ctx, r)
}

func TestPostqueueSnapshotMissingCannotMatchSendCorrection(t *testing.T) {
	root := t.TempDir()
	manager, err := New(root, testGate{})
	if err != nil {
		t.Fatal(err)
	}
	p := &process{state: State{ID: "managed", Status: "running"}}
	manager.processes["managed"] = p
	manager.gate = destinationPolicyFunc(func(_ context.Context, r contract.Request) error {
		if r.Act == "read" {
			manager.mu.Lock()
			delete(manager.processes, "managed")
			manager.mu.Unlock()
			if err := os.Remove(filepath.Join(root, "managed.json")); err != nil {
				t.Error(err)
			}
		}
		return nil
	})
	_, err = manager.Send(context.Background(), "fixture", "managed", "PRIVATE_PROMPT")
	if !errors.Is(err, ErrSessionNotFound) || IsPrequeueDestinationError(err, ErrUnmanaged) {
		t.Fatal("late missing snapshot became send correction")
	}
	if len(p.state.Pending) != 1 {
		t.Fatal("postqueue uncertainty lost pending work")
	}
	if _, statErr := os.Stat(filepath.Join(root, "managed.events.jsonl")); statErr != nil {
		t.Fatal("expected queued event absent")
	}
}
