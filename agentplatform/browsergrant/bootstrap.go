package browsergrant

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

type Attempt struct {
	Offer, ServerContribution, ClientContribution, Transcript        map[string]any
	OfferDigest, ServerCommitment, ClientCommitment, BootstrapDigest string
	EditorConfirmation, CompanionConfirmation                        *Envelope
	Expires                                                          int64
	State                                                            string
}

func nonce() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func newID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func keyID(k []byte) string { b := sha256.Sum256(k); return hex.EncodeToString(b[:]) }
func (a *Authority) Offer(ctx context.Context, instance, window string) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, errors.New("authority closed")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	now := a.now().UnixMilli()
	for id, v := range a.attempts {
		if v.Expires <= now || v.State == "aborted" {
			delete(a.attempts, id)
			delete(a.pairs, id)
		}
	}
	if len(a.attempts) >= 32 {
		return nil, errors.New("pending bootstrap limit")
	}
	key := instance + "/" + window
	recent := []int64{}
	for _, t := range a.starts[key] {
		if t > now-300000 {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 3 {
		return nil, errors.New("bootstrap attempt rate limit")
	}
	a.starts[key] = append(recent, now)
	pair := newID()
	pub := a.key.Public().(ed25519.PublicKey)
	offer := map[string]any{"version": 2, "pair": pair, "authority": a.incarnation, "serverKey": base64.RawURLEncoding.EncodeToString(pub), "serverKeyId": keyID(pub), "instance": instance, "windowLease": window, "issuedAt": now, "expires": now + 60000}
	od, e := Digest("bootstrap-offer", offer)
	if e != nil {
		return nil, e
	}
	contribution := cloneMap(offer)
	contribution["role"] = "server"
	contribution["serverNonce"] = nonce()
	commit, e := Digest("bootstrap-commit", map[string]any{"version": 2, "role": "server", "offerDigest": od, "contribution": contribution})
	if e != nil {
		return nil, e
	}
	a.attempts[pair] = &Attempt{Offer: offer, ServerContribution: contribution, OfferDigest: od, ServerCommitment: commit, Expires: now + 60000, State: "offer-verified"}
	return map[string]any{"offer": cloneMap(offer), "offerDigest": od, "serverCommitment": commit}, nil
}
func (a *Authority) attempt(pair, instance, window string) (*Attempt, error) {
	v, ok := a.attempts[pair]
	if a.closed || !ok || v.Expires <= a.now().UnixMilli() || v.Offer["instance"] != instance || v.Offer["windowLease"] != window || v.State == "aborted" {
		return nil, errors.New("inactive exact bootstrap")
	}
	return v, nil
}
func (a *Authority) Commit(pair, instance, window, offerDigest, commit string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return e
	}
	if v.State != "offer-verified" || offerDigest != v.OfferDigest || len(commit) != 64 {
		a.abort(v)
		return errors.New("commitment mismatch or replay")
	}
	if _, e = hex.DecodeString(commit); e != nil {
		a.abort(v)
		return e
	}
	v.ClientCommitment = commit
	v.State = "commitments-fixed"
	return nil
}
func (a *Authority) Reveal(pair, instance, window, offerDigest string, c map[string]any) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return nil, e
	}
	deny := func() (map[string]any, error) {
		a.abort(v)
		return nil, errors.New("reveal mismatch or replay")
	}
	if v.State != "commitments-fixed" || offerDigest != v.OfferDigest || !exactFields(c, []string{"version", "role", "pair", "client", "clientKey", "clientKeyId", "tabId", "document", "origin", "clientNonce"}) || c["version"] != 2 || c["role"] != "companion" || c["pair"] != pair || c["origin"] != "https://iugum-agent-code.local" {
		return deny()
	}
	cd, e := Digest("bootstrap-commit", map[string]any{"version": 2, "role": "companion", "offerDigest": offerDigest, "contribution": c})
	if e != nil || cd != v.ClientCommitment {
		return deny()
	}
	pub, e := unbase64(stringField(c, "clientKey"), 32)
	if e != nil || len(pub) != 32 || keyID(pub) != c["clientKeyId"] {
		return deny()
	}
	n, e := unbase64(stringField(c, "clientNonce"), 32)
	if e != nil || len(n) != 32 {
		return deny()
	}
	if !protocolID.MatchString(stringField(c, "client")) || !protocolID.MatchString(stringField(c, "document")) {
		return deny()
	}
	tab, ok := c["tabId"].(int64)
	if !ok || tab < 1 || tab > 9007199254740991 {
		return deny()
	}
	transcript := cloneMap(v.ServerContribution)
	delete(transcript, "role")
	for k, x := range c {
		if k == "role" {
			continue
		}
		if old, exists := transcript[k]; exists && old != x {
			return deny()
		}
		transcript[k] = x
	}
	digest, e := Digest("bootstrap", transcript)
	if e != nil {
		return deny()
	}
	v.ClientContribution = cloneMap(c)
	v.Transcript = transcript
	v.BootstrapDigest = digest
	v.State = "compared"
	return map[string]any{"serverContribution": cloneMap(v.ServerContribution), "transcript": cloneMap(transcript), "bootstrapDigest": digest, "state": v.State}, nil
}
func stringField(v map[string]any, key string) string { s, _ := v[key].(string); return s }
func (a *Authority) ConfirmEditor(pair, instance, window, digest string, confirmed bool) (Envelope, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return Envelope{}, e
	}
	if !confirmed || v.State != "compared" || v.EditorConfirmation != nil || digest != v.BootstrapDigest {
		a.abort(v)
		return Envelope{}, errors.New("editor confirmation mismatch")
	}
	r := map[string]any{"version": 2, "pair": pair, "bootstrapDigest": digest, "role": "editor", "confirmedAt": a.now().UnixMilli()}
	b, e := SignedBytes("confirmation", r)
	if e != nil {
		return Envelope{}, e
	}
	envelope := Envelope{r, base64.RawURLEncoding.EncodeToString(ed25519.Sign(a.key, b))}
	v.EditorConfirmation = &envelope
	a.activate(v)
	return Envelope{cloneMap(r), envelope.Signature}, nil
}
func (a *Authority) ConfirmCompanion(pair, instance, window string, envelope Envelope) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return e
	}
	r := envelope.Record
	if v.State != "compared" || v.CompanionConfirmation != nil || !exactFields(r, []string{"version", "pair", "bootstrapDigest", "role", "confirmedAt"}) || r["version"] != 2 || r["pair"] != pair || r["bootstrapDigest"] != v.BootstrapDigest || r["role"] != "companion" {
		a.abort(v)
		return errors.New("companion confirmation mismatch")
	}
	t, ok := r["confirmedAt"].(int64)
	if !ok || t > a.now().UnixMilli()+100 || t < v.Offer["issuedAt"].(int64)-100 {
		return errors.New("confirmation time")
	}
	b, e := SignedBytes("confirmation", r)
	if e != nil {
		return e
	}
	pub, _ := base64.RawURLEncoding.DecodeString(stringField(v.ClientContribution, "clientKey"))
	sig, e := unbase64(envelope.Signature, 64)
	if e != nil || !ed25519.Verify(pub, b, sig) {
		a.abort(v)
		return errors.New("companion confirmation signature")
	}
	v.CompanionConfirmation = &Envelope{cloneMap(r), envelope.Signature}
	a.activate(v)
	return nil
}
func (a *Authority) activate(v *Attempt) {
	if v.EditorConfirmation == nil || v.CompanionConfirmation == nil {
		return
	}
	t := v.Transcript
	pub, _ := base64.RawURLEncoding.DecodeString(stringField(t, "clientKey"))
	a.pairs[stringField(t, "pair")] = Pair{ID: stringField(t, "pair"), Authority: a.incarnation, ServerKeyID: stringField(t, "serverKeyId"), ClientKeyID: stringField(t, "clientKeyId"), Instance: stringField(t, "instance"), WindowLease: stringField(t, "windowLease"), Client: stringField(t, "client"), Document: stringField(t, "document"), Origin: stringField(t, "origin"), TabID: t["tabId"].(int64), Expires: v.Expires, ClientKey: pub}
	v.State = "active"
}
func (a *Authority) Status(pair, instance, window string) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return nil, e
	}
	r := map[string]any{"state": v.State, "pair": pair, "expires": v.Expires}
	if v.EditorConfirmation != nil {
		r["editorConfirmation"] = *v.EditorConfirmation
	}
	if v.CompanionConfirmation != nil {
		r["companionConfirmation"] = *v.CompanionConfirmation
	}
	return r, nil
}
func (a *Authority) Cancel(pair, instance, window string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, e := a.attempt(pair, instance, window)
	if e != nil {
		return e
	}
	a.abort(v)
	delete(a.pairs, pair)
	for id, proof := range a.observations {
		if proof["pair"] == pair {
			delete(a.observations, id)
		}
	}
	return nil
}

func (a *Authority) abort(v *Attempt) {
	v.State = "aborted"
	delete(a.pairs, stringField(v.Offer, "pair"))
	for id, p := range a.observations {
		if p["pair"] == v.Offer["pair"] {
			delete(a.observations, id)
		}
	}
}
