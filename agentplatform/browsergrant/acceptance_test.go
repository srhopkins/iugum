package browsergrant

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

type ackFixture struct {
	a        *Authority
	d        *Dispatcher
	key      ed25519.PrivateKey
	e        Envelope
	viewport map[string]any
	targets  []any
	raster   []byte
}

func newACKFixture(t *testing.T) ackFixture {
	t.Helper()
	a, key, e, v, targets, raster := observationFixture(t)
	d, err := NewDispatcher(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	id := e.Record["commandId"].(string)
	d.store.Commands[id] = &DispatchCommand{Pair: "pair", CommandID: id, Operation: "observe", Args: map[string]any{}, Instance: "instance", Window: "lease", State: "granted", Expires: a.now().Add(30 * time.Second).UnixMilli()}
	return ackFixture{a, d, key, e, v, targets, raster}
}
func (f ackFixture) observe() (Envelope, error) {
	return f.d.AcceptObservationACKForWindow(context.Background(), "instance", "lease", f.e, f.viewport, f.targets, f.raster)
}
func (f ackFixture) terminal(t *testing.T, lifecycle string) (Envelope, map[string]any) {
	t.Helper()
	r := map[string]any{}
	for _, k := range common {
		r[k] = f.e.Record[k]
	}
	r["observation"] = ""
	r["viewportDigest"] = f.e.Record["viewportDigest"]
	r["targetDigest"], _ = Digest("target", nil)
	r["lifecycle"] = lifecycle
	r["finishedAt"] = f.a.now().UnixMilli()
	r["errorCode"] = ""
	var data map[string]any
	if lifecycle == "completed" {
		oid := f.e.Record["observation"].(string)
		data = map[string]any{"operation": "observe", "observation": oid, "observationDigest": f.a.observations[oid]["digest"]}
	} else {
		r["errorCode"] = "transport"
	}
	var value any
	if data != nil {
		value = data
	}
	r["dataDigest"], _ = Digest("result-data", value)
	return signEnvelope(t, f.key, "result", r), data
}
func verifyACK(t *testing.T, a *Authority, ack Envelope, kind string, submitted Envelope) {
	t.Helper()
	if len(ack.Record) != 22 {
		t.Fatal("wrong acceptance schema", ack.Record)
	}
	if ack.Record["kind"] != kind {
		t.Fatal("wrongkind")
	}
	digest, _ := Digest(kind, submitted.Record)
	if ack.Record["requestDigest"] != digest {
		t.Fatal("wrong submitted digest")
	}
	b, _ := SignedBytes("acceptance", ack.Record)
	sig, err := base64.RawURLEncoding.DecodeString(ack.Signature)
	if err != nil || !ed25519.Verify(a.key.Public().(ed25519.PublicKey), b, sig) {
		t.Fatal("invalid serverACKsignature")
	}
	for _, domain := range []string{"grant", "clock-probe", "observation", "result", "confirmation"} {
		b, _ = SignedBytes(domain, ack.Record)
		if ed25519.Verify(a.key.Public().(ed25519.PublicKey), b, sig) {
			t.Fatal("crossdomainACK", domain)
		}
	}
	if ack.Record["expires"].(int64) <= ack.Record["acceptedAt"].(int64) || ack.Record["expires"].(int64) > ack.Record["acceptedAt"].(int64)+1000 {
		t.Fatal("ACKdeadline")
	}
}
func TestAcceptanceActualDurableBothKindsOneUseAndReadiness(t *testing.T) {
	f := newACKFixture(t)
	ack, err := f.observe()
	if err != nil {
		t.Fatal(err)
	}
	verifyACK(t, f.a, ack, "observation", f.e)
	oid := f.e.Record["observation"].(string)
	if ack.Record["originalObservation"] != "" || ack.Record["acceptedObservation"] != oid || ack.Record["lifecycle"] != "accepted" {
		t.Fatal("observation binding")
	}
	if f.a.observations[oid]["ready"] != false || f.d.latest["pair"] != "" {
		t.Fatal("observation ACK promoted readiness")
	}
	if _, err = f.observe(); err == nil {
		t.Fatal("observation ACKreissued")
	}
	terminal, data := f.terminal(t, "completed")
	result, err := f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", terminal, data)
	if err != nil {
		t.Fatal(err)
	}
	verifyACK(t, f.a, result, "result", terminal)
	if result.Record["originalObservation"] != "" || result.Record["acceptedObservation"] != oid || result.Record["lifecycle"] != "completed" {
		t.Fatal("new/original observation swapped")
	}
	if f.d.latest["pair"] != oid || f.a.observations[oid]["ready"] != true {
		t.Fatal("completed ACK missing actualproof")
	}
	if _, err = f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", terminal, data); err == nil {
		t.Fatal("terminal ACKreissued")
	}
	b, _ := os.ReadFile(f.a.store)
	var receipts map[string]Receipt
	json.Unmarshal(b, &receipts)
	if receipts[terminal.Record["commandId"].(string)].State != "completed" {
		t.Fatal("ACKbeforecorepersist")
	}
	b, _ = os.ReadFile(f.d.path)
	var commands dispatchStore
	json.Unmarshal(b, &commands)
	if commands.Commands[terminal.Record["commandId"].(string)].State != "completed" {
		t.Fatal("ACKbeforedispatchpersist")
	}
}
func TestAcceptanceUncertainUsesOriginalReceiptAfterDetach(t *testing.T) {
	f := newACKFixture(t)
	if _, err := f.observe(); err != nil {
		t.Fatal(err)
	}
	terminal, data := f.terminal(t, "uncertain")
	ack, err := f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", terminal, data)
	if err != nil {
		t.Fatal(err)
	}
	verifyACK(t, f.a, ack, "result", terminal)
	if _, ok := f.a.pairs["pair"]; ok {
		t.Fatal("uncertain retainedpair")
	}
	if ack.Record["lifecycle"] != "uncertain" || ack.Record["instance"] != "instance" || ack.Record["acceptedObservation"] != f.e.Record["observation"] {
		t.Fatal("detached original receipt bindinglost")
	}
	if f.d.latest["pair"] != "" {
		t.Fatal("uncertain proofready")
	}
}
func TestAcceptanceStorageFailureOrInvalidClientNeverSigns(t *testing.T) {
	for _, mode := range []string{"core-storage", "dispatcher-storage", "invalid-client", "closed"} {
		t.Run(mode, func(t *testing.T) {
			f := newACKFixture(t)
			count := 0
			original := f.a.sign
			f.a.sign = func(b []byte) ([]byte, error) { count++; return original(b) }
			if mode == "dispatcher-storage" {
				if _, err := f.observe(); err != nil {
					t.Fatal(err)
				}
				count = 0
				f.d.save = func(dispatchStore) error { return os.ErrPermission }
				terminal, data := f.terminal(t, "completed")
				if ack, err := f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", terminal, data); err == nil || ack.Signature != "" {
					t.Fatal("dispatcher failure ACK")
				}
			} else {
				if mode == "core-storage" {
					f.a.save = func(map[string]Receipt) error { return os.ErrPermission }
				}
				if mode == "invalid-client" {
					f.e.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
				}
				if mode == "closed" {
					f.d.Close()
				}
				if ack, err := f.observe(); err == nil || ack.Signature != "" {
					t.Fatal("invalid acceptance ACK")
				}
			}
			if count != 0 {
				t.Fatal("failure reachedsigning", count)
			}
		})
	}
}
func TestAcceptanceSigningOrDeadlineFailureConsumedNoReissue(t *testing.T) {
	for _, mode := range []string{"sign", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			f := newACKFixture(t)
			original := f.a.sign
			count := 0
			f.a.sign = func(b []byte) ([]byte, error) {
				count++
				if mode == "sign" {
					return nil, errors.New("synthetic signing failure")
				}
				sig, err := original(b)
				future := f.a.now().Add(2 * time.Second)
				f.a.now = func() time.Time { return future }
				return sig, err
			}
			if ack, err := f.observe(); err == nil || ack.Signature != "" {
				t.Fatal("uncertain ACKemitted")
			}
			if _, err := f.observe(); err == nil {
				t.Fatal("failedACKtransition retried")
			}
			if count != 1 {
				t.Fatal("ACKsignreissued", count)
			}
			if f.a.receipts[f.e.Record["commandId"].(string)].Observation == "" {
				t.Fatal("failedACKrolledbackconsumedtransition")
			}
			if _, ok := f.a.pairs["pair"]; ok {
				t.Fatal("failedACKdidnotdetach")
			}
		})
	}
}
func TestAcceptanceSharedCanonicalVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/acceptance-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Records []struct {
			Domain, SubmittedDomain, Canonical, Digest, PublicKey, Signature string
			Record, SubmittedRecord                                          map[string]any
		}
	}
	if err = json.Unmarshal(b, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Records) != 2 {
		t.Fatal("bothkinds required")
	}
	for _, v := range vectors.Records {
		c, err := Canonical(v.Record)
		if err != nil || string(c) != v.Canonical {
			t.Fatal("canonical vector mismatch")
		}
		digest, _ := Digest(v.Domain, v.Record)
		if digest != v.Digest {
			t.Fatal("digest vector mismatch")
		}
		rd, _ := Digest(v.SubmittedDomain, v.SubmittedRecord)
		if v.Record["requestDigest"] != rd {
			t.Fatal("requestdigest vector mismatch")
		}
		pub, _ := base64.RawURLEncoding.DecodeString(v.PublicKey)
		sig, _ := base64.RawURLEncoding.DecodeString(v.Signature)
		signed, _ := SignedBytes("acceptance", v.Record)
		if !ed25519.Verify(pub, signed, sig) {
			t.Fatal("signature vector mismatch")
		}
	}
}

func TestAcceptanceObservationCapsConservativeProofExpiry(t *testing.T) {
	f := newACKFixture(t)
	fixed := f.a.now()
	f.a.now = func() time.Time { return fixed }
	f.e.Record["capturedAt"] = fixed.UnixMilli()
	f.e.Record["expires"] = fixed.Add(350 * time.Millisecond).UnixMilli()
	f.e = signEnvelope(t, f.key, "observation", f.e.Record)
	ack, err := f.observe()
	if err != nil {
		t.Fatal(err)
	}
	if ack.Record["expires"] != fixed.Add(250*time.Millisecond).UnixMilli() {
		t.Fatal("ACKpassedconservativeproofexpiry", ack.Record)
	}
}
func TestAcceptanceInputOriginalObservationAndEmptyAcceptedCapture(t *testing.T) {
	f := newACKFixture(t)
	if _, err := f.observe(); err != nil {
		t.Fatal(err)
	}
	terminal, data := f.terminal(t, "completed")
	if _, err := f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", terminal, data); err != nil {
		t.Fatal(err)
	}
	oid := f.e.Record["observation"].(string)
	q := Request{Pair: "pair", CommandID: newID(), Operation: "click", Observation: oid, Args: map[string]any{"x": float64(.5), "y": float64(.5), "target": "button"}}
	if _, err := f.d.QueueForWindow(context.Background(), "instance", "lease", q); err != nil {
		t.Fatal(err)
	}
	if _, err := f.d.PollForWindow(context.Background(), "pair", "instance", "lease"); err != nil {
		t.Fatal(err)
	}
	grant, err := f.d.GrantForWindow(context.Background(), "instance", "lease", q)
	if err != nil {
		t.Fatal(err)
	}
	receipt := f.a.receipts[q.CommandID]
	r := map[string]any{}
	for _, k := range common {
		r[k] = grant.Record[k]
	}
	r["grantDigest"] = receipt.GrantDigest
	r["viewportDigest"] = receipt.Proof["viewportDigest"]
	r["targetDigest"], _ = Digest("target", f.targets[0])
	inputData := map[string]any{"operation": "click", "dispatched": true}
	r["dataDigest"], _ = Digest("result-data", inputData)
	r["lifecycle"] = "completed"
	r["finishedAt"] = f.a.now().UnixMilli()
	r["errorCode"] = ""
	signed := signEnvelope(t, f.key, "result", r)
	ack, err := f.d.AcceptResultACKForWindow(context.Background(), "instance", "lease", signed, inputData)
	if err != nil {
		t.Fatal(err)
	}
	verifyACK(t, f.a, ack, "result", signed)
	if ack.Record["originalObservation"] != oid || ack.Record["acceptedObservation"] != "" {
		t.Fatal("inputacceptedcapture fabricated", ack.Record)
	}
}

func TestAcceptanceBindsValidatedOwnedCopyNotMutableCaller(t *testing.T) {
	f := newACKFixture(t)
	expected := cloneMap(f.e.Record)
	raster := append([]byte(nil), f.raster...)
	original := f.a.save
	f.a.save = func(receipts map[string]Receipt) error {
		f.e.Record["client"] = "caller-mutated-after-validation"
		f.raster[0] = 0
		return original(receipts)
	}
	ack, err := f.observe()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Digest("observation", expected)
	if ack.Record["requestDigest"] != want || ack.Record["client"] != expected["client"] {
		t.Fatal("ACKsignedpostvalidationcallerdata")
	}
	got := f.d.images[expected["observation"].(string)].Raster
	if len(got) != len(raster) || got[0] != raster[0] {
		t.Fatal("cached unvalidated mutated caller raster")
	}
}

func TestAcceptanceCancelledAfterDurableAcceptNoACKOrRetry(t *testing.T) {
	f := newACKFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := f.a.save
	count := 0
	sign := f.a.sign
	f.a.save = func(v map[string]Receipt) error { err := original(v); cancel(); return err }
	f.a.sign = func(b []byte) ([]byte, error) { count++; return sign(b) }
	ack, err := f.d.AcceptObservationACKForWindow(ctx, "instance", "lease", f.e, f.viewport, f.targets, f.raster)
	if err == nil || ack.Signature != "" {
		t.Fatal("cancelled acceptanceACK")
	}
	if count != 0 {
		t.Fatal("cancelled acceptance signed")
	}
	if f.a.receipts[f.e.Record["commandId"].(string)].Observation == "" {
		t.Fatal("cancelled response rolledbackacceptedtransition")
	}
	if _, err = f.observe(); err == nil {
		t.Fatal("cancelled acceptedtransition retry")
	}
}
