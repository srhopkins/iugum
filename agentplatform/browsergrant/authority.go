package browsergrant

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
)

// Authority owns the policy writer lock. Do not pass policy.Live or a callback:
// external policy-file writers are not serialized with grant acceptance.
// Normal Server construction leaves this disabled; explicit source flags opt in.
// Source review/deployment is separate from proving an actual browser client.
type Authority struct {
	mu                        sync.Mutex
	storeLock                 *flock.Flock
	closed                    bool
	health                    *storeHealth
	attempts                  map[string]*Attempt
	starts                    map[string][]int64
	gate                      *policy.Gate
	revision                  uint64
	actor, store, incarnation string
	key                       ed25519.PrivateKey
	sign                      func([]byte) ([]byte, error)
	receipts                  map[string]Receipt
	pairs                     map[string]Pair
	observations              map[string]map[string]any
	now                       func() time.Time
	save                      func(map[string]Receipt) error
}
type Pair struct {
	ID, Authority, ServerKeyID, ClientKeyID, Instance, WindowLease, Client, Document, Origin string
	TabID                                                                                    int64
	Expires                                                                                  int64
	ClientKey                                                                                ed25519.PublicKey
}
type Receipt struct {
	GrantDigest    string         `json:"grantDigest"`
	PolicyRevision uint64         `json:"policyRevision"`
	State          string         `json:"state"`
	Pair           string         `json:"pair"`
	Record         map[string]any `json:"record"`
	Args           map[string]any `json:"args"`
	Proof          map[string]any `json:"proof,omitempty"`
	Observation    string         `json:"observation,omitempty"`
}
type Request struct {
	Pair, CommandID, Operation, Observation string
	Args                                    map[string]any
}
type Envelope struct {
	Record    map[string]any `json:"record"`
	Signature string         `json:"signature"`
}

func New(store, actor, incarnation string, gate *policy.Gate) (*Authority, error) {
	if gate == nil || store == "" || actor == "" || incarnation == "" {
		return nil, errors.New("explicit policy, private store and authority identity required")
	}
	absolute, e := filepath.Abs(store)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(absolute), 0700); e != nil {
		return nil, e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(absolute))
	if e != nil {
		return nil, e
	}
	store = filepath.Join(parent, filepath.Base(absolute))
	lock := flock.New(store + ".lock")
	locked, e := lock.TryLock()
	if e != nil || !locked {
		_ = lock.Close()
		return nil, errors.New("browser receipt store already owned or lock unavailable")
	}
	success := false
	defer func() {
		if !success {
			_ = lock.Close()
		}
	}()
	health, e := openStoreHealth(store)
	if e != nil {
		return nil, e
	}
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, e
	}
	a := &Authority{health: health, storeLock: lock, gate: gate, revision: 1, actor: actor, store: store, incarnation: incarnation, key: key, receipts: map[string]Receipt{}, pairs: map[string]Pair{}, now: time.Now, observations: map[string]map[string]any{}, attempts: map[string]*Attempt{}, starts: map[string][]int64{}}
	if b, e := os.ReadFile(store); e == nil {
		if e = json.Unmarshal(b, &a.receipts); e != nil {
			return nil, e
		}
		if a.receipts == nil {
			return nil, errors.New("null receipt store")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	for id, r := range a.receipts {
		if r.State == "reserved" || r.State == "issued" {
			r.State = "uncertain"
			a.receipts[id] = r
		}
	}
	a.sign = func(b []byte) ([]byte, error) { return ed25519.Sign(a.key, b), nil }
	a.save = a.saveFile
	if e = a.persist(); e != nil {
		return nil, e
	}
	success = true
	return a, nil
}
func (a *Authority) saveFile(v map[string]Receipt) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(a.store), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(a.store), ".browser-receipts-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), a.store); e != nil {
		return e
	}
	d, e := os.Open(filepath.Dir(a.store))
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// ReplacePolicy is the only supported writer/revoke boundary. The supplied
// immutable actual Casbin gate must not subsequently be mutated or shared.
func (a *Authority) ReplacePolicy(g *policy.Gate) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("authority closed")
	}
	if g == nil {
		return errors.New("missing actual policy")
	}
	a.gate = g
	a.revision++
	return nil
}

// RegisterTrustedPair is privileged bootstrap integration only. A public HTTP
// handler MUST NOT map page JSON to this method. Ceremony implementation pending.
func (a *Authority) RegisterTrustedPair(p Pair) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("authority closed")
	}
	if p.Authority != a.incarnation || p.ID == "" || p.Instance == "" || p.WindowLease == "" || p.Client == "" || p.Document == "" || p.Origin != "https://iugum-agent-code.local" || p.TabID < 1 || len(p.ClientKey) != 32 || p.Expires <= a.now().UnixMilli() || p.Expires-a.now().UnixMilli() > 60000 {
		return errors.New("invalid trusted pair")
	}
	p.ClientKey = append(ed25519.PublicKey{}, p.ClientKey...)
	a.pairs[p.ID] = p
	return nil
}
func (a *Authority) Grant(ctx context.Context, q Request) (Envelope, error) {
	return a.grant(ctx, "", "", q)
}
func (a *Authority) grant(ctx context.Context, instance, generation string, q Request) (Envelope, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return Envelope{}, errors.New("browser authority closed")
	}
	if e := ctx.Err(); e != nil {
		return Envelope{}, e
	}
	p, ok := a.pairs[q.Pair]
	now := a.now().UnixMilli()
	if !ok || p.Authority != a.incarnation || p.Expires <= now || instance != "" && (p.Instance != instance || p.WindowLease != generation) {
		return Envelope{}, errors.New("inactive trusted pair")
	}
	if !protocolID.MatchString(q.CommandID) || q.Observation != "" && !protocolID.MatchString(q.Observation) {
		return Envelope{}, errors.New("missing command UUID")
	}
	if len(a.receipts) >= 4096 {
		return Envelope{}, errors.New("durable browser receipt capacity reached")
	}
	if _, ok = a.receipts[q.CommandID]; ok {
		return Envelope{}, errors.New("command already consumed")
	}
	args, e := arguments(q.Operation, q.Args)
	if e != nil {
		return Envelope{}, e
	}
	if q.Operation == "observe" && q.Observation != "" || q.Operation != "observe" && q.Observation == "" {
		return Envelope{}, errors.New("observation binding required")
	}
	if e = a.gate.Enforce(ctx, contract.Request{Sub: a.actor, Obj: "agent/browser/" + p.ID + "/" + q.Operation, Act: "write"}); e != nil {
		return Envelope{}, e
	}
	// Context/time checked after actual Casbin under the same policy writer lock.
	if e = ctx.Err(); e != nil {
		return Envelope{}, e
	}
	now = a.now().UnixMilli()
	if now >= p.Expires {
		return Envelope{}, errors.New("pair expired before reservation")
	}
	if q.Operation != "observe" {
		proof, ok := a.observations[q.Observation]
		if !ok || proof["pair"] != p.ID || proof["ready"] != true || proof["expires"].(int64) <= now {
			return Envelope{}, errors.New("validated fresh observation required")
		}
		if q.Operation == "click" {
			found := false
			for _, v := range proof["targets"].([]any) {
				t := v.(map[string]any)
				if t["id"] == args["target"] {
					b := t["bounds"].(map[string]any)
					x, y := args["x"].(float64), args["y"].(float64)
					found = x >= b["x"].(float64) && y >= b["y"].(float64) && x < b["x"].(float64)+b["width"].(float64) && y < b["y"].(float64)+b["height"].(float64)
				}
			}
			if !found {
				return Envelope{}, errors.New("click target does not match validated observation")
			}
		}
	}
	digest, e := Digest("arguments", map[string]any{"operation": q.Operation, "args": args})
	if e != nil {
		return Envelope{}, e
	}
	expires := now + 1000
	if expires > p.Expires {
		expires = p.Expires
	}
	r := map[string]any{"version": 2, "pair": p.ID, "authority": p.Authority, "serverKeyId": p.ServerKeyID, "clientKeyId": p.ClientKeyID, "instance": p.Instance, "windowLease": p.WindowLease, "client": p.Client, "document": p.Document, "tabId": p.TabID, "origin": p.Origin, "commandId": q.CommandID, "operation": q.Operation, "argsDigest": digest, "observation": q.Observation, "issuedAt": now, "expires": expires}
	b, e := SignedBytes("grant", r)
	if e != nil {
		return Envelope{}, e
	}
	gd, e := Digest("grant", r)
	if e != nil {
		return Envelope{}, e
	}
	receipt := Receipt{PolicyRevision: a.revision, GrantDigest: gd, State: "reserved", Pair: p.ID, Record: r, Args: args}
	if q.Operation != "observe" {
		receipt.Proof = cloneMap(a.observations[q.Observation])
		delete(a.observations, q.Observation)
	}
	a.receipts[q.CommandID] = receipt // Tombstone remains even on unknown storage outcome.
	if e = a.persist(); e != nil {
		receipt.State = "uncertain"
		a.receipts[q.CommandID] = receipt
		return Envelope{}, e
	}
	signature, signError := a.sign(b)
	if signError != nil || len(signature) != ed25519.SignatureSize {
		receipt.State = "uncertain"
		a.receipts[q.CommandID] = receipt
		_ = a.persist()
		return Envelope{}, errors.New("grant signing failed; UUID consumed")
	}
	receipt.State = "issued"
	a.receipts[q.CommandID] = receipt
	if e = a.persist(); e != nil {
		receipt.State = "uncertain"
		a.receipts[q.CommandID] = receipt
		return Envelope{}, e
	}
	if e = ctx.Err(); e != nil || a.now().UnixMilli() >= expires {
		receipt.State = "uncertain"
		a.receipts[q.CommandID] = receipt
		_ = a.persist()
		return Envelope{}, errors.New("grant expired/cancelled during durable signing")
	}
	return Envelope{cloneMap(r), base64.RawURLEncoding.EncodeToString(signature)}, nil
}
func arguments(op string, args map[string]any) (map[string]any, error) {
	keys := map[string][]string{"observe": {}, "click": {"x", "y", "target"}, "scroll": {"deltaX", "deltaY"}}
	want, ok := keys[op]
	if !ok || len(args) != len(want) {
		return nil, errors.New("unsupported typed operation")
	}
	for _, k := range want {
		if _, ok = args[k]; !ok {
			return nil, errors.New("unknown argument")
		}
	}
	if op == "click" {
		t, ok := args["target"].(string)
		if !ok || len(t) < 1 || len(t) > 128 {
			return nil, errors.New("invalid target")
		}
		for _, c := range t {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				return nil, errors.New("invalid target")
			}
		}
	}
	for k, v := range args {
		if k == "target" {
			continue
		}
		n, ok := v.(float64)
		if !ok {
			return nil, errors.New("typed numeric argument required")
		}
		if _, e := Canonical(n); e != nil {
			return nil, e
		}
		if op == "click" && n < 0 || op == "scroll" && (n < -2000 || n > 2000) {
			return nil, errors.New("out of bounds")
		}
	}
	if op == "scroll" && args["deltaX"] == float64(0) && args["deltaY"] == float64(0) {
		return nil, errors.New("empty scroll")
	}
	out := map[string]any{}
	for k, v := range args {
		out[k] = v
	}
	return out, nil
}

// GrantForWindow binds the actual private registry lease, not caller-selected
// pair identity. Adapter holds registry lock through this serialized operation.
func (a *Authority) GrantForWindow(ctx context.Context, instance, generation string, q Request) (Envelope, error) {
	return a.grant(ctx, instance, generation, q)
}

func (a *Authority) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	a.attempts = map[string]*Attempt{}
	a.starts = map[string][]int64{}
	a.pairs = map[string]Pair{}
	a.observations = map[string]map[string]any{}
	healthError := a.health.close()
	lockError := a.storeLock.Close()
	return errors.Join(healthError, lockError)
}
