package browsergrant

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"testing"
	"time"
)

func TestClockProbeNonceDomainAndOneUse(t *testing.T) {
	a, d := dispatcher(t)
	challenge := nonce()
	e, err := d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if e.Record["probeNonce"] != challenge || e.Record["operation"] != "observe" {
		t.Fatal("clock binding")
	}
	sig, _ := base64.RawURLEncoding.DecodeString(e.Signature)
	pub := a.key.Public().(ed25519.PublicKey)
	b, _ := SignedBytes("clock-probe", e.Record)
	if !ed25519.Verify(pub, b, sig) {
		t.Fatal("clock signature")
	}
	grantBytes, _ := SignedBytes("grant", e.Record)
	if ed25519.Verify(pub, grantBytes, sig) {
		t.Fatal("clock proof is input authority")
	}
	swapped := cloneMap(e.Record)
	swapped["probeNonce"] = nonce()
	b, _ = SignedBytes("clock-probe", swapped)
	if ed25519.Verify(pub, b, sig) {
		t.Fatal("early proof accepted different fresh nonce")
	}
	if _, err = d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", nonce()); err == nil {
		t.Fatal("second probe attempt allowed")
	}
	receipt := a.receipts[e.Record["commandId"].(string)]
	if receipt.State != "clock-probe" {
		t.Fatal("clock UUID not consumed")
	}
	if _, err = d.GrantForWindow(context.Background(), "instance", "lease", Request{Pair: "pair", CommandID: e.Record["commandId"].(string), Operation: "observe", Args: map[string]any{}}); err == nil {
		t.Fatal("clock UUID bypassed real queue")
	}
}
func TestClockProbeFailureConsumesAttemptAndQuarantinesStorage(t *testing.T) {
	a, d := dispatcher(t)
	d.save = func(dispatchStore) error { return os.ErrPermission }
	if _, e := d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", nonce()); e == nil {
		t.Fatal("storage failure emitted clockproof")
	}
	if len(a.receipts) != 0 {
		t.Fatal("failed queue persistence reached signing")
	}
	if _, e := d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", nonce()); e == nil {
		t.Fatal("failed attempt retried")
	}
	if e := d.Close(); e == nil {
		t.Fatal("failed store marked clean")
	}
	if b, e := NewDispatcher(a); e == nil {
		b.Close()
		t.Fatal("restart forgot failed probe attempt")
	}
}

func TestClockProbePolicyDenialAndExpiredSigningNeverRetries(t *testing.T) {
	for _, mode := range []string{"policy", "expiry", "sign"} {
		t.Run(mode, func(t *testing.T) {
			a, d := dispatcher(t)
			if mode == "policy" {
				a.ReplacePolicy(gate(t, false))
			}
			if mode == "sign" {
				a.sign = func([]byte) ([]byte, error) { return nil, os.ErrPermission }
			}
			if mode == "expiry" {
				original := a.sign
				a.sign = func(b []byte) ([]byte, error) {
					now := a.now().Add(2 * time.Second)
					a.now = func() time.Time { return now }
					return original(b)
				}
			}
			if _, err := d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", nonce()); err == nil {
				t.Fatal("invalid proof emitted")
			}
			if len(d.store.Probes) != 1 {
				t.Fatal("failed attempt not consumed")
			}
			if _, err := d.ClockProbeForWindow(context.Background(), "pair", "instance", "lease", nonce()); err == nil {
				t.Fatal("failed probe retried")
			}
		})
	}
}
