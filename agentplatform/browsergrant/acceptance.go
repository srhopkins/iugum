package browsergrant

import (
	"context"
	"encoding/base64"
	"errors"
)

// AcceptObservationACKForWindow consumes the actual validated observation
// transition before issuing its sole receipt. It cannot sign arbitrary records.
func (d *Dispatcher) AcceptObservationACKForWindow(ctx context.Context, instance, window string, e Envelope, viewport map[string]any, targets []any, raster []byte) (Envelope, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e = Envelope{Record: cloneMap(e.Record), Signature: e.Signature}
	viewport = cloneMap(viewport)
	targets = cloneArray(targets)
	raster = append([]byte(nil), raster...)
	expiry, err := d.originalPairExpiry(e)
	if err != nil {
		return Envelope{}, err
	}
	if err = d.acceptObservationLocked(ctx, instance, window, e, viewport, targets, raster); err != nil {
		return Envelope{}, err
	}
	return d.acceptanceLocked(ctx, "observation", e.Record, expiry)
}

// AcceptResultACKForWindow preserves accepted terminal state even if signing or
// response delivery fails. The consumed transition can never reissue an ACK.
func (d *Dispatcher) AcceptResultACKForWindow(ctx context.Context, instance, window string, e Envelope, data map[string]any) (Envelope, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e = Envelope{Record: cloneMap(e.Record), Signature: e.Signature}
	if data != nil {
		data = cloneMap(data)
	}
	expiry, err := d.originalPairExpiry(e)
	if err != nil {
		return Envelope{}, err
	}
	if err = d.acceptResultLocked(ctx, instance, window, e, data); err != nil {
		return Envelope{}, err
	}
	return d.acceptanceLocked(ctx, "result", e.Record, expiry)
}

// Caller holds d.mu. The expiry is taken from the original registered pair
// before an uncertain terminal transition can detach it.
func (d *Dispatcher) originalPairExpiry(e Envelope) (int64, error) {
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if d.closed || !d.health.healthy || a.closed || !a.health.healthy {
		return 0, errors.New("acceptance authority unavailable")
	}
	id, _ := e.Record["commandId"].(string)
	receipt, ok := a.receipts[id]
	if !ok {
		return 0, errors.New("original issued receipt required")
	}
	pair, ok := a.pairs[receipt.Pair]
	if !ok {
		return 0, errors.New("original active pair required")
	}
	return pair.Expires, nil
}

// This helper is reachable only after an actual one-use acceptance transition
// while d.mu is held. It has no exported signing/recovery endpoint.
func (d *Dispatcher) acceptanceLocked(ctx context.Context, kind string, submitted map[string]any, pairExpiry int64) (Envelope, error) {
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	id, _ := submitted["commandId"].(string)
	receipt, ok := a.receipts[id]
	c := d.store.Commands[id]
	fail := func(message string) (Envelope, error) {
		if ok {
			a.detachPair(receipt.Pair)
			delete(d.latest, receipt.Pair)
		}
		return Envelope{}, errors.New(message)
	}
	if d.closed || !d.health.healthy || a.closed || !a.health.healthy || ctx.Err() != nil || !ok || c == nil {
		return fail("acceptance signing unavailable; never retry")
	}
	original := receipt.Record
	record := map[string]any{}
	for _, key := range []string{"version", "pair", "authority", "serverKeyId", "clientKeyId", "instance", "windowLease", "client", "document", "tabId", "origin", "commandId", "operation", "argsDigest"} {
		record[key] = original[key]
	}
	record["grantDigest"] = receipt.GrantDigest
	record["originalObservation"] = original["observation"]
	record["acceptedObservation"] = receipt.Observation
	record["kind"] = kind
	digest, err := Digest(kind, submitted)
	if err != nil {
		return fail("accepted record digest failed")
	}
	record["requestDigest"] = digest
	now := a.now().UnixMilli()
	expires := now + 1000
	if kind == "observation" {
		proof := a.observations[receipt.Observation]
		if receipt.State != "issued" || c.State != "granted" || proof == nil || proof["ready"] != false || d.images[receipt.Observation] == nil {
			return fail("accepted observation transition unavailable")
		}
		record["lifecycle"] = "accepted"
		if cap := proof["expires"].(int64); expires > cap {
			expires = cap
		}
		if expires > pairExpiry {
			expires = pairExpiry
		}
	} else if kind == "result" {
		if receipt.State != c.State || receipt.State != submitted["lifecycle"] || (receipt.State != "completed" && receipt.State != "rejected" && receipt.State != "uncertain") {
			return fail("accepted terminal transition unavailable")
		}
		record["lifecycle"] = receipt.State
		if expires > pairExpiry+5000 {
			expires = pairExpiry + 5000
		}
	} else {
		return fail("unsupported acceptance kind")
	}
	if expires <= now {
		return fail("acceptance expiry exhausted; never retry")
	}
	record["acceptedAt"] = now
	record["expires"] = expires
	bytes, err := SignedBytes("acceptance", record)
	if err != nil {
		return fail("acceptance canonical signing failed")
	}
	signature, err := a.sign(bytes)
	if err != nil || len(signature) != 64 || ctx.Err() != nil || a.now().UnixMilli() >= expires || a.closed || !a.health.healthy || !d.health.healthy {
		return fail("acceptance signing uncertain; never retry")
	}
	return Envelope{Record: record, Signature: base64.RawURLEncoding.EncodeToString(signature)}, nil
}
