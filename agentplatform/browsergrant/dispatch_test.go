package browsergrant

import (
	"context"
	"testing"
	"time"
)

func dispatcher(t *testing.T) (*Authority, *Dispatcher) {
	t.Helper()
	a := authority(t)
	d, e := NewDispatcher(a)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { d.Close() })
	return a, d
}
func TestQueueDoesNotMintEarlyGrantAndPollNeverRepeats(t *testing.T) {
	a, d := dispatcher(t)
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	ctx := context.Background()
	if _, e := d.QueueForWindow(ctx, "instance", "lease", q); e != nil {
		t.Fatal(e)
	}
	if len(a.receipts) != 0 {
		t.Fatal("queue minted grant")
	}
	fixed := a.now().Add(2 * time.Second)
	a.now = func() time.Time { return fixed }
	c, e := d.PollForWindow(ctx, "pair", "instance", "lease")
	if e != nil || c == nil || c.CommandID != commandOne {
		t.Fatal(c, e)
	}
	if c, e := d.PollForWindow(ctx, "pair", "instance", "lease"); e != nil || c != nil {
		t.Fatal("replayed delivery", c, e)
	}
	if len(a.receipts) != 0 {
		t.Fatal("poll minted grant")
	}
	env, e := d.GrantForWindow(ctx, "instance", "lease", q)
	if e != nil {
		t.Fatal(e)
	}
	if env.Record["issuedAt"] != fixed.UnixMilli() {
		t.Fatal("grant issued before actual execution request")
	}
	if _, e := d.GrantForWindow(ctx, "instance", "lease", q); e == nil {
		t.Fatal("grant request replayed")
	}
}
func TestGrantRequiresExactRealQueuedCommand(t *testing.T) {
	_, d := dispatcher(t)
	ctx := context.Background()
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := d.GrantForWindow(ctx, "instance", "lease", q); e == nil {
		t.Fatal("arbitrary page UUID granted")
	}
	d.QueueForWindow(ctx, "instance", "lease", q)
	d.PollForWindow(ctx, "pair", "instance", "lease")
	wrong := q
	wrong.CommandID = newID()
	if _, e := d.GrantForWindow(ctx, "instance", "lease", wrong); e == nil {
		t.Fatal("changed UUID granted")
	}
	wrong = q
	wrong.Operation = "scroll"
	wrong.Observation = newID()
	wrong.Args = map[string]any{"deltaX": float64(0), "deltaY": float64(1)}
	if _, e := d.GrantForWindow(ctx, "instance", "lease", wrong); e == nil {
		t.Fatal("changed queued operation granted")
	}
	if _, e := d.GrantForWindow(ctx, "instance", "other-window", q); e == nil {
		t.Fatal("changed editor window granted")
	}
}
func TestRestartNeverRedeliversQueuedOrDispatched(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "dispatched"}[delivered], func(t *testing.T) {
			a, d := dispatcher(t)
			q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
			if _, e := d.QueueForWindow(context.Background(), "instance", "lease", q); e != nil {
				t.Fatal(e)
			}
			if delivered {
				d.PollForWindow(context.Background(), "pair", "instance", "lease")
			}
			d.Close()
			b, e := NewDispatcher(a)
			if e != nil {
				t.Fatal(e)
			}
			defer b.Close()
			if b.store.Commands[commandOne].State != "uncertain" || b.store.Commands[commandOne].Instance != "instance" || b.store.Commands[commandOne].Window != "lease" {
				t.Fatal("restart lost provenance or redelivered")
			}
			if _, e = b.QueueForWindow(context.Background(), "instance", "lease", q); e == nil {
				t.Fatal("restart reused queued UUID")
			}
		})
	}
}

func TestQueuedCommandRechecksActualPolicyAtDeliveryAndGrant(t *testing.T) {
	for _, stage := range []string{"poll", "grant"} {
		t.Run(stage, func(t *testing.T) {
			a, d := dispatcher(t)
			ctx := context.Background()
			q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
			if _, e := d.QueueForWindow(ctx, "instance", "lease", q); e != nil {
				t.Fatal(e)
			}
			if stage == "grant" {
				if _, e := d.PollForWindow(ctx, "pair", "instance", "lease"); e != nil {
					t.Fatal(e)
				}
			}
			if e := a.ReplacePolicy(gate(t, false)); e != nil {
				t.Fatal(e)
			}
			if stage == "poll" {
				if c, e := d.PollForWindow(ctx, "pair", "instance", "lease"); e == nil || c != nil {
					t.Fatal("revoked queue delivered")
				}
			} else {
				if _, e := d.GrantForWindow(ctx, "instance", "lease", q); e == nil {
					t.Fatal("revoked grant issued")
				}
			}
			if len(a.receipts) != 0 {
				t.Fatal("revoked grant reserved or signed")
			}
		})
	}
}
func TestQueueCancellationAndPostCloseNeverReplays(t *testing.T) {
	_, d := dispatcher(t)
	ctx := context.Background()
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := d.QueueForWindow(ctx, "instance", "lease", q); e != nil {
		t.Fatal(e)
	}
	if e := d.CancelForWindow(ctx, "pair", "instance", "lease", commandOne); e != nil {
		t.Fatal(e)
	}
	if c, e := d.PollForWindow(ctx, "pair", "instance", "lease"); e != nil || c != nil {
		t.Fatal("cancelled queue delivered", e)
	}
	if _, e := d.QueueForWindow(ctx, "instance", "lease", q); e == nil {
		t.Fatal("cancelled UUID reused")
	}
	if e := d.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := d.ClockProbeForWindow(ctx, "pair", "instance", "lease", nonce()); e == nil {
		t.Fatal("closed clock mutation")
	}
	if _, e := d.QueueForWindow(ctx, "instance", "lease", Request{Pair: "pair", CommandID: newID(), Operation: "observe", Args: map[string]any{}}); e == nil {
		t.Fatal("closed queue mutation")
	}
}
