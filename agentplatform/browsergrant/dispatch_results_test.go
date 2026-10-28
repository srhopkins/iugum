package browsergrant

import (
	"context"
	"testing"
	"time"
)

func TestDispatchSignedObservationTerminalFreshnessAndMutationFence(t *testing.T) {
	a, key, observation, viewport, targets, raster := observationFixture(t)
	d, err := NewDispatcher(a)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	id := observation.Record["commandId"].(string)
	// The shared fixture has already obtained a real Authority grant. Seed only
	// its matching private queue provenance to isolate result/image lifecycle.
	d.store.Commands[id] = &DispatchCommand{Pair: "pair", CommandID: id, Operation: "observe", Args: map[string]any{}, Instance: "instance", Window: "lease", State: "granted", Expires: a.now().Add(30 * time.Second).UnixMilli()}
	if err = d.AcceptObservationForWindow(ctx, "other-window", "lease", observation, viewport, targets, raster); err == nil {
		t.Fatal("foreign window image accepted")
	}
	if err = d.AcceptObservationForWindow(ctx, "instance", "lease", observation, viewport, targets, raster); err != nil {
		t.Fatal(err)
	}
	oid := observation.Record["observation"].(string)
	if d.latest["pair"] != "" {
		t.Fatal("unfinished observe advertised ready")
	}
	terminal := map[string]any{}
	for _, k := range common {
		terminal[k] = observation.Record[k]
	}
	terminal["observation"] = ""
	terminal["viewportDigest"] = observation.Record["viewportDigest"]
	terminal["targetDigest"], _ = Digest("target", nil)
	data := map[string]any{"operation": "observe", "observation": oid, "observationDigest": a.observations[oid]["digest"]}
	terminal["dataDigest"], _ = Digest("result-data", data)
	terminal["lifecycle"] = "completed"
	terminal["finishedAt"] = a.now().UnixMilli()
	terminal["errorCode"] = ""
	if err = d.AcceptResultForWindow(ctx, "instance", "lease", signEnvelope(t, key, "result", terminal), data); err != nil {
		t.Fatal(err)
	}
	if d.latest["pair"] != oid {
		t.Fatal("completed signed observe missing readiness")
	}
	// Queueing input needs the exact current proof; final grant consumes it.
	q := Request{Pair: "pair", CommandID: newID(), Operation: "click", Observation: oid, Args: map[string]any{"x": float64(.5), "y": float64(.5), "target": "button"}}
	if _, err = d.QueueForWindow(ctx, "instance", "lease", q); err != nil {
		t.Fatal(err)
	}
	if _, err = d.PollForWindow(ctx, "pair", "instance", "lease"); err != nil {
		t.Fatal(err)
	}
	if _, err = d.GrantForWindow(ctx, "instance", "lease", q); err != nil {
		t.Fatal(err)
	}
	if d.latest["pair"] != "" || a.observations[oid] != nil {
		t.Fatal("input left old image authorized")
	}
	if err = d.AcceptResultForWindow(ctx, "instance", "lease", signEnvelope(t, key, "result", terminal), data); err == nil {
		t.Fatal("terminal replay accepted")
	}
}
