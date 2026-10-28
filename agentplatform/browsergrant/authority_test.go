package browsergrant

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"github.com/srhopkins/iugum/policy"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const commandOne = "11111111-1111-4111-8111-111111111111"

func gate(t *testing.T, allow bool) *policy.Gate {
	t.Helper()
	p := filepath.Join(t.TempDir(), "policy.csv")
	s := "p, tester, agent/browser/pair/observe, write, allow\n"
	if !allow {
		s = "p, tester, *, *, deny\n"
	}
	if e := os.WriteFile(p, []byte(s), 0600); e != nil {
		t.Fatal(e)
	}
	g, e := policy.New("", p)
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func authority(t *testing.T) *Authority {
	t.Helper()
	a, e := New(filepath.Join(t.TempDir(), "receipts.json"), "tester", "incarnation", gate(t, true))
	if e != nil {
		t.Fatal(e)
	}
	_, client, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	e = a.RegisterTrustedPair(Pair{ID: "pair", Authority: "incarnation", ServerKeyID: "server", ClientKeyID: "clientkey", Instance: "instance", WindowLease: "lease", Client: "client", Document: "document", Origin: "https://iugum-agent-code.local", TabID: 17, Expires: time.Now().Add(time.Minute).UnixMilli(), ClientKey: client.Public().(ed25519.PublicKey)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
func TestActualCasbinRevokeBeforeReservationDenies(t *testing.T) {
	a := authority(t)
	if e := a.ReplacePolicy(gate(t, false)); e != nil {
		t.Fatal(e)
	}
	if _, e := a.Grant(context.Background(), Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}); e == nil {
		t.Fatal("revoked actual policy granted")
	}
	if len(a.receipts) != 0 {
		t.Fatal("deny reserved")
	}
}
func TestSignedGrantAndConcurrentOneUse(t *testing.T) {
	a := authority(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := a.Grant(context.Background(), Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}})
			if e == nil {
				mu.Lock()
				count++
				mu.Unlock()
				b, e := SignedBytes("grant", v.Record)
				sig, _ := base64.RawURLEncoding.DecodeString(v.Signature)
				if e != nil || !ed25519.Verify(a.key.Public().(ed25519.PublicKey), b, sig) {
					t.Error("invalid signature")
				}
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatalf("issued%d", count)
	}
}
func TestStorageFailureConsumesUUID(t *testing.T) {
	a := authority(t)
	a.save = func(map[string]Receipt) error { return os.ErrPermission }
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := a.Grant(context.Background(), q); e == nil {
		t.Fatal("storage error signed")
	}
	if a.receipts[commandOne].State != "uncertain" {
		t.Fatal("not uncertain")
	}
	if _, e := a.Grant(context.Background(), q); e == nil {
		t.Fatal("storage failure replayed")
	}
}
func TestRestartNeverReissuesAndDoesNotRestorePair(t *testing.T) {
	a := authority(t)
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := a.Grant(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	a.Close()
	b, e := New(a.store, a.actor, "different-incarnation", gate(t, true))
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.receipts[commandOne].State != "uncertain" || len(b.pairs) != 0 {
		t.Fatal("restart restores authority")
	}
	if _, e = b.Grant(context.Background(), q); e == nil {
		t.Fatal("restart replay")
	}
}
func TestContextAndExpiryBeforeGrantDeny(t *testing.T) {
	a := authority(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := a.Grant(ctx, q); e == nil {
		t.Fatal("cancelled grant")
	}
	a.now = func() time.Time { return time.Now().Add(time.Hour) }
	if _, e := a.Grant(context.Background(), q); e == nil {
		t.Fatal("expired grant")
	}
}
func TestSigningAndSecondDurableWriteFailureNeverReplay(t *testing.T) {
	for _, kind := range []string{"sign", "issued-write", "late-storage"} {
		t.Run(kind, func(t *testing.T) {
			a := authority(t)
			save := a.save
			n := 0
			if kind == "sign" {
				a.sign = func([]byte) ([]byte, error) { return nil, os.ErrPermission }
			} else {
				a.save = func(v map[string]Receipt) error {
					n++
					if kind == "issued-write" && n == 2 {
						return os.ErrPermission
					}
					if kind == "late-storage" && n == 1 {
						old := a.now()
						a.now = func() time.Time { return old.Add(2 * time.Second) }
					}
					return save(v)
				}
			}
			q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
			if _, e := a.Grant(context.Background(), q); e == nil {
				t.Fatal("ambiguous failure emitted grant")
			}
			if a.receipts[commandOne].State != "uncertain" {
				t.Fatal("failure not uncertain")
			}
			if _, e := a.Grant(context.Background(), q); e == nil {
				t.Fatal("failure replayed")
			}
		})
	}
}
func TestExclusiveLifetimeStoreLockAndRelease(t *testing.T) {
	a := authority(t)
	var wg sync.WaitGroup
	success := 0
	var mu sync.Mutex
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, e := New(a.store, a.actor, "other-incarnation", a.gate)
			if e == nil {
				mu.Lock()
				success++
				mu.Unlock()
				b.Close()
			}
		}()
	}
	wg.Wait()
	if success != 0 {
		t.Fatal("second constructor acquired live receipt store")
	}
	q := Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}
	if _, e := a.Grant(context.Background(), q); e != nil {
		t.Fatal(e)
	}
	if e := a.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := New(a.store, a.actor, "other-incarnation", a.gate)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.receipts[commandOne].State != "uncertain" {
		t.Fatal("released restart lost tombstone")
	}
	if _, e = a.Grant(context.Background(), Request{Pair: "pair", CommandID: newID(), Operation: "observe", Args: map[string]any{}}); e == nil {
		t.Fatal("closed authority still granted")
	}
}
func TestAllMutatorsAfterCloseCannotWriteNewOwnersStore(t *testing.T) {
	a := authority(t)
	p := a.pairs["pair"]
	a.Close()
	b, e := New(a.store, a.actor, "new-incarnation", a.gate)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	before, e := os.ReadFile(a.store)
	if e != nil {
		t.Fatal(e)
	}
	writes := 0
	a.save = func(map[string]Receipt) error { writes++; return nil }
	ctx := context.Background()
	checks := []error{a.ReplacePolicy(a.gate), a.RegisterTrustedPair(p), a.AcceptObservation(ctx, Envelope{}, nil, nil, nil), a.AcceptObservationForWindow(ctx, "i", "w", Envelope{}, nil, nil, nil), a.AcceptResult(ctx, Envelope{}, nil), a.AcceptResultForWindow(ctx, "i", "w", Envelope{}, nil), a.Commit("p", "i", "w", "d", "c"), a.ConfirmCompanion("p", "i", "w", Envelope{}), a.Cancel("p", "i", "w")}
	for _, e := range checks {
		if e == nil {
			t.Fatal("closed mutator accepted")
		}
	}
	if _, e = a.Offer(ctx, "i", "w"); e == nil {
		t.Fatal("closed offer")
	}
	if _, e = a.Reveal("p", "i", "w", "d", nil); e == nil {
		t.Fatal("closed reveal")
	}
	if _, e = a.ConfirmEditor("p", "i", "w", "d", true); e == nil {
		t.Fatal("closed confirmation")
	}
	if _, e = a.Grant(ctx, Request{}); e == nil {
		t.Fatal("closed grant")
	}
	if writes != 0 || len(a.attempts) != 0 || len(a.pairs) != 0 {
		t.Fatal("closed owner mutated")
	}
	after, e := os.ReadFile(a.store)
	if e != nil || string(before) != string(after) {
		t.Fatal("closed owner changed new owner store")
	}
}
func TestFailedReservationQuarantinesRestartWithoutHistoryReset(t *testing.T) {
	a := authority(t)
	a.save = func(map[string]Receipt) error { return os.ErrPermission }
	if _, e := a.Grant(context.Background(), Request{Pair: "pair", CommandID: commandOne, Operation: "observe", Args: map[string]any{}}); e == nil {
		t.Fatal("failed write emitted grant")
	}
	if e := a.Close(); e == nil {
		t.Fatal("unhealthy Close claimed clean")
	}
	if b, e := New(a.store, a.actor, "restart", a.gate); e == nil {
		b.Close()
		t.Fatal("restart lost unpersisted UUID and resumed")
	}
	if _, e := os.Stat(a.store); e != nil {
		t.Fatal("quarantine reset receipt history", e)
	}
}
func TestCrashMarkerQuarantinesAndHealthyCloseAllowsRestart(t *testing.T) {
	a := authority(t)
	if e := a.storeLock.Close(); e != nil {
		t.Fatal(e)
	} // Simulates OS releasing lock on abrupt process death; NO graceful clean marker.
	if b, e := New(a.store, a.actor, "restart", a.gate); e == nil {
		b.Close()
		t.Fatal("unclean restart resumed")
	}
	a.health.healthy = false
	a.Close()
	b := authority(t)
	if e := b.Close(); e != nil {
		t.Fatal(e)
	}
	c, e := New(b.store, b.actor, "restart", b.gate)
	if e != nil {
		t.Fatal("clean restart denied", e)
	}
	c.Close()
}
