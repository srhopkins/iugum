package browsergrant

import (
	"context"
	"errors"
)

// DispatchImage holds a bounded, validated observation only in memory. Raster
// bytes are never written into the durable command ledger.
type DispatchImage struct {
	Record   map[string]any
	Viewport map[string]any
	Targets  []any
	Raster   []byte
	Fresh    bool
}

func (d *Dispatcher) AcceptObservationForWindow(ctx context.Context, instance, window string, e Envelope, viewport map[string]any, targets []any, raster []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.acceptObservationLocked(ctx, instance, window, e, viewport, targets, raster)
}
func (d *Dispatcher) acceptObservationLocked(ctx context.Context, instance, window string, e Envelope, viewport map[string]any, targets []any, raster []byte) error {
	if err := d.expirePending(); err != nil {
		return err
	}
	id, _ := e.Record["commandId"].(string)
	c := d.store.Commands[id]
	if d.closed || c == nil || c.State != "granted" || c.Instance != instance || c.Window != window || c.Pair != e.Record["pair"] {
		return errors.New("actual granted observation command required")
	}
	if err := d.a.AcceptObservationForWindow(ctx, instance, window, e, viewport, targets, raster); err != nil {
		return err
	}
	oid, _ := e.Record["observation"].(string)
	// One pending command per pair bounds current images; evict historical images
	// separately from durable UUID tombstones. Raster bytes never enter the ledger.
	if len(d.images) >= 4 {
		for key := range d.images {
			delete(d.images, key)
			break
		}
	}
	d.images[oid] = &DispatchImage{Record: cloneMap(e.Record), Viewport: cloneMap(viewport), Targets: cloneArray(targets), Raster: append([]byte(nil), raster...)}
	return nil
}
func (d *Dispatcher) AcceptResultForWindow(ctx context.Context, instance, window string, e Envelope, data map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.acceptResultLocked(ctx, instance, window, e, data)
}
func (d *Dispatcher) acceptResultLocked(ctx context.Context, instance, window string, e Envelope, data map[string]any) error {
	if err := d.expirePending(); err != nil {
		return err
	}
	id, _ := e.Record["commandId"].(string)
	c := d.store.Commands[id]
	if d.closed || c == nil || c.State != "granted" || c.Instance != instance || c.Window != window || c.Pair != e.Record["pair"] {
		return errors.New("actual granted terminal command required")
	}
	if err := d.a.AcceptResultForWindow(ctx, instance, window, e, data); err != nil {
		return err
	}
	c.State, _ = e.Record["lifecycle"].(string)
	c.Result = cloneMap(data)
	c.Error, _ = e.Record["errorCode"].(string)
	delete(d.latest, c.Pair)
	if c.State == "completed" && c.Operation == "observe" {
		oid, _ := data["observation"].(string)
		d.latest[c.Pair] = oid
	}
	if err := d.persist(); err != nil {
		c.State = "uncertain"
		delete(d.latest, c.Pair)
		d.a.mu.Lock()
		d.a.detachPair(c.Pair)
		d.a.mu.Unlock()
		return err
	}
	return nil
}
func (d *Dispatcher) CancelForWindow(ctx context.Context, pair, instance, window, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.expirePending(); err != nil {
		return err
	}
	if _, err := d.binding(ctx, pair, instance, window, ""); err != nil {
		return err
	}
	c := d.store.Commands[id]
	if c == nil || c.Pair != pair || c.Instance != instance || c.Window != window {
		return errors.New("exact queued command required")
	}
	switch c.State {
	case "queued":
		c.State = "cancelled"
	case "dispatched", "granted":
		c.State = "uncertain"
		d.a.mu.Lock()
		d.a.detachPair(pair)
		d.a.mu.Unlock()
	default:
		return errors.New("terminal command cannot be cancelled")
	}
	c.Error = "cancelled; command is never replayed"
	delete(d.latest, pair)
	return d.persist()
}

// ImageForWindow returns only already verified raster data. Status metadata
// decides freshness independently; an old result never authorizes another input.
func (d *Dispatcher) ImageForWindow(ctx context.Context, pair, instance, window, id string) (*DispatchImage, error) {
	if _, err := d.StatusForWindow(ctx, pair, instance, window, id); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.store.Commands[id]
	if c == nil || c.State != "completed" || c.Operation != "observe" {
		return nil, nil
	}
	oid, _ := c.Result["observation"].(string)
	image := d.images[oid]
	if image == nil {
		return nil, nil
	}
	d.a.mu.Lock()
	proof := d.a.observations[oid]
	fresh := !d.a.closed && d.latest[pair] == oid && proof != nil && proof["ready"] == true && proof["expires"].(int64) > d.a.now().UnixMilli()
	d.a.mu.Unlock()
	return &DispatchImage{Fresh: fresh, Record: cloneMap(image.Record), Viewport: cloneMap(image.Viewport), Targets: cloneArray(image.Targets), Raster: append([]byte(nil), image.Raster...)}, nil
}

// expirePending consumes timed-out delivery UUIDs. An issued or delivered action
// cannot be recalled; its pair is detached and the outcome remains uncertain.
// Caller holds d.mu; mutation order is always dispatcher then authority.
func (d *Dispatcher) expirePending() error {
	if d.closed || !d.health.healthy {
		return errors.New("dispatcher unavailable")
	}
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || !a.health.healthy {
		return errors.New("authority closed or quarantined")
	}
	changed := false
	for _, c := range d.store.Commands {
		if (c.State == "queued" || c.State == "dispatched" || c.State == "granted") && c.Expires <= a.now().UnixMilli() {
			old := c.State
			c.State = "uncertain"
			c.Error = "command delivery/result deadline expired; never replay"
			changed = true
			delete(d.latest, c.Pair)
			if old != "queued" {
				a.detachPair(c.Pair)
			}
		}
	}
	if changed {
		return d.persist()
	}
	return nil
}
