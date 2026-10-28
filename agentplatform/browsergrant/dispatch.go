package browsergrant

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"github.com/srhopkins/iugum/contract"
	"os"
	"sort"
	"sync"
	"time"
)

// Dispatcher is source-only until the optional Server integration is reviewed.
// It keeps unsent commands separate from the execution-time grant ledger.
type Dispatcher struct {
	mu     sync.Mutex
	a      *Authority
	path   string
	lock   *flock.Flock
	health *storeHealth
	closed bool
	save   func(dispatchStore) error
	store  dispatchStore
	images map[string]*DispatchImage
	latest map[string]string
}
type dispatchStore struct {
	Commands map[string]*DispatchCommand `json:"commands"`
	Probes   map[string]*ClockAttempt    `json:"probes"`
}
type DispatchCommand struct {
	Pair        string         `json:"pair"`
	CommandID   string         `json:"commandId"`
	Operation   string         `json:"operation"`
	Args        map[string]any `json:"args"`
	Observation string         `json:"observation"`
	Instance    string         `json:"instance"`
	Window      string         `json:"windowGeneration"`
	State       string         `json:"state"`
	Expires     int64          `json:"expires"`
	Error       string         `json:"error,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
}
type ClockAttempt struct{ Pair, CommandID, Nonce, State string }

func NewDispatcher(a *Authority) (*Dispatcher, error) {
	if a == nil {
		return nil, errors.New("actual authority required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, errors.New("authority closed")
	}
	path := a.store + ".dispatch"
	lock := flock.New(path + ".lock")
	ok, e := lock.TryLock()
	if e != nil || !ok {
		_ = lock.Close()
		return nil, errors.New("browser dispatcher already owned")
	}
	success := false
	defer func() {
		if !success {
			_ = lock.Close()
		}
	}()
	health, e := openStoreHealth(path)
	if e != nil {
		return nil, e
	}
	d := &Dispatcher{images: map[string]*DispatchImage{}, latest: map[string]string{}, a: a, path: path, lock: lock, health: health, store: dispatchStore{Commands: map[string]*DispatchCommand{}, Probes: map[string]*ClockAttempt{}}}
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &d.store); e != nil {
			return nil, e
		}
		if d.store.Commands == nil || d.store.Probes == nil {
			return nil, errors.New("invalid browser dispatch store")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	for _, c := range d.store.Commands {
		if c == nil {
			return nil, errors.New("invalid queued command")
		}
		if c.State == "queued" || c.State == "dispatched" || c.State == "granted" {
			c.State = "uncertain"
			c.Error = "service restarted; queued browser action is never replayed"
		}
	}
	for _, p := range d.store.Probes {
		if p == nil {
			return nil, errors.New("invalid clock attempt")
		}
		if p.State != "consumed" {
			p.State = "uncertain"
		}
	}
	d.save = d.saveFile
	if e = d.persist(); e != nil {
		return nil, e
	}
	success = true
	return d, nil
}
func (d *Dispatcher) saveFile(v dispatchStore) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return atomicHealth(d.path, b)
}
func (d *Dispatcher) persist() error {
	e := d.save(d.store)
	if e != nil {
		d.health.healthy = false
	}
	return e
}
func (d *Dispatcher) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return errors.Join(d.health.close(), d.lock.Close())
}
func (d *Dispatcher) binding(ctx context.Context, pair, instance, window string, operation string) (Pair, error) {
	if d.closed || !d.health.healthy {
		return Pair{}, errors.New("dispatcher closed or quarantined")
	}
	a := d.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || !a.health.healthy {
		return Pair{}, errors.New("authority closed or quarantined")
	}
	if e := ctx.Err(); e != nil {
		return Pair{}, e
	}
	p, ok := a.pairs[pair]
	if !ok || p.Instance != instance || p.WindowLease != window || p.Authority != a.incarnation || p.Expires <= a.now().UnixMilli() {
		return Pair{}, errors.New("exact active browser/window pair required")
	}
	if operation != "" {
		if e := a.gate.Enforce(ctx, contract.Request{Sub: a.actor, Obj: "agent/browser/" + pair + "/" + operation, Act: "write"}); e != nil {
			return Pair{}, e
		}
	}
	return p, nil
}
func (d *Dispatcher) QueueForWindow(ctx context.Context, instance, window string, q Request) (DispatchCommand, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.expirePending(); err != nil {
		return DispatchCommand{}, err
	}
	p, e := d.binding(ctx, q.Pair, instance, window, q.Operation)
	if e != nil {
		return DispatchCommand{}, e
	}
	args, e := arguments(q.Operation, q.Args)
	if e != nil {
		return DispatchCommand{}, e
	}
	if !protocolID.MatchString(q.CommandID) || q.Observation != "" && !protocolID.MatchString(q.Observation) || q.Operation == "observe" && q.Observation != "" || q.Operation != "observe" && q.Observation == "" {
		return DispatchCommand{}, errors.New("exact typed command identity required")
	}
	if len(d.store.Commands)+len(d.store.Probes) >= 4096 {
		return DispatchCommand{}, errors.New("dispatch receipt capacity reached")
	}
	if _, ok := d.store.Commands[q.CommandID]; ok {
		return DispatchCommand{}, errors.New("queued UUID already consumed")
	}
	for _, p := range d.store.Probes {
		if p.CommandID == q.CommandID {
			return DispatchCommand{}, errors.New("probe UUID already consumed")
		}
	}
	d.a.mu.Lock()
	defer d.a.mu.Unlock()
	// Serialize actual policy/context/lease acceptance with the durable queue
	// reservation, just as Authority does at the final execution grant.
	if err := ctx.Err(); err != nil {
		return DispatchCommand{}, err
	}
	current, ok := d.a.pairs[q.Pair]
	if !ok || current.Instance != instance || current.WindowLease != window || current.Expires <= d.a.now().UnixMilli() || d.a.closed {
		return DispatchCommand{}, errors.New("pair changed before reservation")
	}
	if err := d.a.gate.Enforce(ctx, contract.Request{Sub: d.a.actor, Obj: "agent/browser/" + q.Pair + "/" + q.Operation, Act: "write"}); err != nil {
		return DispatchCommand{}, err
	}
	if q.Operation != "observe" {
		proof := d.a.observations[q.Observation]
		if proof == nil || proof["pair"] != q.Pair || proof["ready"] != true || proof["expires"].(int64) <= d.a.now().UnixMilli() || d.latest[q.Pair] != q.Observation {
			return DispatchCommand{}, errors.New("fresh completed observation required")
		}
	}
	_, used := d.a.receipts[q.CommandID]
	if used {
		return DispatchCommand{}, errors.New("authority UUID already consumed")
	}
	for _, c := range d.store.Commands {
		if c.Pair == q.Pair && (c.State == "queued" || c.State == "dispatched" || c.State == "granted") {
			return DispatchCommand{}, errors.New("one browser command pending per pair")
		}
	}
	expires := d.a.now().Add(30 * time.Second).UnixMilli()
	if expires > p.Expires {
		expires = p.Expires
	}
	c := &DispatchCommand{Pair: q.Pair, CommandID: q.CommandID, Operation: q.Operation, Args: args, Observation: q.Observation, Instance: instance, Window: window, State: "queued", Expires: expires}
	if q.Operation == "observe" {
		delete(d.latest, q.Pair)
		for oid, proof := range d.a.observations {
			if proof["pair"] == q.Pair {
				delete(d.a.observations, oid)
			}
		}
	}
	d.store.Commands[q.CommandID] = c
	if e = d.persist(); e != nil {
		c.State = "uncertain"
		return DispatchCommand{}, e
	}
	return cloneCommand(c), nil
}
func cloneCommand(c *DispatchCommand) DispatchCommand {
	v := *c
	v.Args = cloneMap(c.Args)
	if c.Result != nil {
		v.Result = cloneMap(c.Result)
	}
	return v
}
func (d *Dispatcher) PollForWindow(ctx context.Context, pair, instance, window string) (*DispatchCommand, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.expirePending(); err != nil {
		return nil, err
	}
	if _, e := d.binding(ctx, pair, instance, window, ""); e != nil {
		return nil, e
	}
	ids := make([]string, 0, len(d.store.Commands))
	for id := range d.store.Commands {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := d.store.Commands[id]
		if c.Pair != pair || c.Instance != instance || c.Window != window || c.State != "queued" {
			continue
		}
		if c.Expires <= d.a.now().UnixMilli() {
			c.State = "uncertain"
			c.Error = "queue expired without delivery"
			if e := d.persist(); e != nil {
				return nil, e
			}
			continue
		}
		if _, e := d.binding(ctx, pair, instance, window, c.Operation); e != nil {
			c.State = "rejected"
			c.Error = "policy/lease denied before delivery"
			_ = d.persist()
			return nil, e
		}
		c.State = "dispatched"
		if e := d.persist(); e != nil {
			c.State = "uncertain"
			return nil, e
		}
		v := cloneCommand(c)
		return &v, nil
	}
	return nil, nil
}
func (d *Dispatcher) GrantForWindow(ctx context.Context, instance, window string, q Request) (Envelope, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.expirePending(); err != nil {
		return Envelope{}, err
	}
	if _, e := d.binding(ctx, q.Pair, instance, window, ""); e != nil {
		return Envelope{}, e
	}
	c, ok := d.store.Commands[q.CommandID]
	if !ok || c.State != "dispatched" || c.Pair != q.Pair || c.Instance != instance || c.Window != window || c.Expires <= d.a.now().UnixMilli() || c.Operation != q.Operation || c.Observation != q.Observation {
		return Envelope{}, errors.New("actual dispatched command required")
	}
	args, e := arguments(q.Operation, q.Args)
	if e != nil {
		return Envelope{}, e
	}
	want, _ := Canonical(c.Args)
	got, _ := Canonical(args)
	if string(want) != string(got) {
		return Envelope{}, errors.New("queued arguments changed")
	}
	envelope, e := d.a.GrantForWindow(ctx, instance, window, q)
	if e != nil {
		c.State = "uncertain"
		c.Error = "grant denied or outcome uncertain"
		_ = d.persist()
		return Envelope{}, e
	}
	delete(d.latest, c.Pair)
	c.State = "granted"
	if e = d.persist(); e != nil {
		c.State = "uncertain"
		d.a.mu.Lock()
		d.a.detachPair(c.Pair)
		d.a.mu.Unlock()
		return Envelope{}, e
	}
	return envelope, nil
}
func (d *Dispatcher) StatusForWindow(ctx context.Context, pair, instance, window, id string) (DispatchCommand, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.expirePending(); err != nil {
		return DispatchCommand{}, err
	}
	if d.closed {
		return DispatchCommand{}, errors.New("dispatcher closed")
	}
	d.a.mu.Lock()
	defer d.a.mu.Unlock()
	closed := d.a.closed
	gate := d.a.gate
	actor := d.a.actor
	if closed {
		return DispatchCommand{}, errors.New("authority closed")
	}
	if e := gate.Enforce(ctx, contract.Request{Sub: actor, Obj: "agent/browser/" + pair + "/status", Act: "read"}); e != nil {
		return DispatchCommand{}, e
	}
	c, ok := d.store.Commands[id]
	if !ok || c.Pair != pair || c.Instance != instance || c.Window != window {
		return DispatchCommand{}, errors.New("exact queued receipt unavailable")
	}
	return cloneCommand(c), nil
}
