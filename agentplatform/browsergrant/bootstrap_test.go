package browsergrant

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func bootstrap(t *testing.T) (*Authority, map[string]any, ed25519.PrivateKey, map[string]any) {
	t.Helper()
	a := authority(t)
	offer, e := a.Offer(context.Background(), "editor", "generation")
	if e != nil {
		t.Fatal(e)
	}
	o := offer["offer"].(map[string]any)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	c := map[string]any{"version": 2, "role": "companion", "pair": o["pair"], "client": "11111111-1111-4111-8111-111111111111", "clientKey": base64.RawURLEncoding.EncodeToString(pub), "clientKeyId": keyID(pub), "tabId": int64(17), "document": "22222222-2222-4222-8222-222222222222", "origin": "https://iugum-agent-code.local", "clientNonce": nonce()}
	return a, offer, key, c
}
func TestBootstrapRequiresBothTrustedConfirmations(t *testing.T) {
	a, offer, key, c := bootstrap(t)
	o := offer["offer"].(map[string]any)
	pair := o["pair"].(string)
	od := offer["offerDigest"].(string)
	cd, _ := Digest("bootstrap-commit", map[string]any{"version": 2, "role": "companion", "offerDigest": od, "contribution": c})
	if e := a.Commit(pair, "editor", "generation", od, cd); e != nil {
		t.Fatal(e)
	}
	r, e := a.Reveal(pair, "editor", "generation", od, c)
	if e != nil {
		t.Fatal(e)
	}
	digest := r["bootstrapDigest"].(string)
	if _, ok := a.pairs[pair]; ok {
		t.Fatal("reveals activated")
	}
	if _, e = a.ConfirmEditor(pair, "editor", "generation", digest, true); e != nil {
		t.Fatal(e)
	}
	if _, ok := a.pairs[pair]; ok {
		t.Fatal("one confirmation activated")
	}
	record := map[string]any{"version": 2, "pair": pair, "bootstrapDigest": digest, "role": "companion", "confirmedAt": a.now().UnixMilli()}
	bytes, _ := SignedBytes("confirmation", record)
	sig := base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, bytes))
	if e = a.ConfirmCompanion(pair, "editor", "generation", Envelope{record, sig}); e != nil {
		t.Fatal(e)
	}
	if _, ok := a.pairs[pair]; !ok {
		t.Fatal("both confirmations did not activate")
	}
	if e = a.Cancel(pair, "editor", "generation"); e != nil {
		t.Fatal(e)
	}
	if _, ok := a.pairs[pair]; ok {
		t.Fatal("cancel retained pair")
	}
}
func TestBootstrapSwapReplayAndWrongWindowFailClosed(t *testing.T) {
	a, offer, _, c := bootstrap(t)
	o := offer["offer"].(map[string]any)
	pair := o["pair"].(string)
	od := offer["offerDigest"].(string)
	if _, e := a.Reveal(pair, "editor", "generation", od, c); e == nil {
		t.Fatal("reveal before commitments")
	}
	a, offer, _, c = bootstrap(t)
	o = offer["offer"].(map[string]any)
	pair = o["pair"].(string)
	od = offer["offerDigest"].(string)
	cd, _ := Digest("bootstrap-commit", map[string]any{"version": 2, "role": "companion", "offerDigest": od, "contribution": c})
	if e := a.Commit(pair, "editor", "WRONG", od, cd); e == nil {
		t.Fatal("wrong window")
	}
	if e := a.Commit(pair, "editor", "generation", od, cd); e != nil {
		t.Fatal(e)
	}
	c["document"] = "swapped"
	if _, e := a.Reveal(pair, "editor", "generation", od, c); e == nil {
		t.Fatal("swapped reveal")
	}
	if a.attempts[pair].State != "aborted" {
		t.Fatal("swap not aborted")
	}
}
