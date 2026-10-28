package browsergrant

import (
	"context"
	"encoding/base64"
	"errors"
)

// ClockProbeForWindow implements independently accepted4c4b: fresh background
// nonce, backend-owned UUID, actual observe gate, durable consumption and a
// separate signature domain. The internal grant signature never leaves here.
func (d *Dispatcher) ClockProbeForWindow(ctx context.Context, pair, instance, window, nonce string) (Envelope, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, e := d.binding(ctx, pair, instance, window, ""); e != nil {
		return Envelope{}, e
	}
	if _, e := unbase64(nonce, 32); e != nil {
		return Envelope{}, e
	}
	if _, ok := d.store.Probes[pair]; ok {
		return Envelope{}, errors.New("clock attempt already consumed for pair")
	}
	if len(d.store.Probes)+len(d.store.Commands) >= 4096 {
		return Envelope{}, errors.New("dispatch capacity reached")
	}
	attempt := &ClockAttempt{Pair: pair, CommandID: newID(), Nonce: nonce, State: "attempting"}
	d.store.Probes[pair] = attempt
	if e := d.persist(); e != nil {
		attempt.State = "uncertain"
		return Envelope{}, e
	}
	// Real Casbin + original pair/lease + durable UUID acceptance, not a mock flag.
	internal, e := d.a.GrantForWindow(ctx, instance, window, Request{Pair: pair, CommandID: attempt.CommandID, Operation: "observe", Args: map[string]any{}})
	if e != nil {
		attempt.State = "uncertain"
		_ = d.persist()
		return Envelope{}, e
	}
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || !a.health.healthy {
		attempt.State = "uncertain"
		_ = d.persist()
		return Envelope{}, errors.New("authority unavailable during clock proof")
	}
	receipt := a.receipts[attempt.CommandID]
	receipt.State = "clock-probe"
	receipt.Proof = map[string]any{"purpose": "clock-probe", "probeNonce": nonce}
	a.receipts[attempt.CommandID] = receipt
	if e = a.persist(); e != nil {
		attempt.State = "uncertain"
		_ = d.persist()
		a.detachPair(pair)
		return Envelope{}, e
	}
	attempt.State = "consumed"
	if e = d.persist(); e != nil {
		attempt.State = "uncertain"
		a.detachPair(pair)
		return Envelope{}, e
	}
	record := cloneMap(internal.Record)
	record["probeNonce"] = nonce
	bytes, e := SignedBytes("clock-probe", record)
	if e != nil {
		return Envelope{}, e
	}
	signature, e := a.sign(bytes)
	if e != nil || len(signature) != 64 || ctx.Err() != nil || a.now().UnixMilli() >= record["expires"].(int64) {
		attempt.State = "uncertain"
		_ = d.persist()
		a.detachPair(pair)
		return Envelope{}, errors.New("clock proof signing/expiry uncertain; never retry")
	}
	return Envelope{Record: record, Signature: base64.RawURLEncoding.EncodeToString(signature)}, nil
}
