package browsergrant

import (
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
)

var common = []string{"version", "pair", "authority", "serverKeyId", "clientKeyId", "instance", "windowLease", "client", "document", "tabId", "origin", "commandId", "grantDigest", "operation", "argsDigest", "observation"}
var resultFields = append(append([]string{}, common...), "viewportDigest", "targetDigest", "dataDigest", "lifecycle", "finishedAt", "errorCode")

func exactFields(m map[string]any, keys []string) bool {
	if len(m) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// AcceptResult verifies background-signed fixed typed data and one terminal
// durable receipt. It accepts no browser-provided arbitrary driver return value.
func (a *Authority) AcceptResult(ctx context.Context, envelope Envelope, data map[string]any) error {
	return a.acceptResult(ctx, "", "", envelope, data)
}
func (a *Authority) AcceptResultForWindow(ctx context.Context, instance, generation string, envelope Envelope, data map[string]any) error {
	return a.acceptResult(ctx, instance, generation, envelope, data)
}
func (a *Authority) acceptResult(ctx context.Context, instance, generation string, envelope Envelope, data map[string]any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("authority closed")
	}
	r := envelope.Record
	if !exactFields(r, resultFields) {
		return errors.New("unexpected result fields")
	}
	id, ok := r["commandId"].(string)
	if !ok {
		return errors.New("invalid command")
	}
	receipt, ok := a.receipts[id]
	if !ok || receipt.State != "issued" {
		return errors.New("no outstanding issued command")
	}
	p, ok := a.pairs[receipt.Pair]
	if !ok || instance != "" && (p.Instance != instance || p.WindowLease != generation) {
		return errors.New("pair authority unavailable")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	now := a.now().UnixMilli()
	if now > p.Expires+5000 {
		return errors.New("receipt grace expired")
	}
	for _, key := range common {
		switch key {
		case "grantDigest":
			if r[key] != receipt.GrantDigest {
				return errors.New("grant digest mismatch")
			}
		case "observation": // observe creates a NEW observation, but terminal result still binds original empty observation.
			if !reflect.DeepEqual(r[key], receipt.Record[key]) {
				return errors.New("original observation mismatch")
			}
		default:
			if !reflect.DeepEqual(r[key], receipt.Record[key]) {
				return errors.New("receipt identity mismatch")
			}
		}
	}
	bytes, e := SignedBytes("result", r)
	if e != nil {
		return e
	}
	sig, e := unbase64(envelope.Signature, 64)
	if e != nil || len(sig) != 64 || !ed25519.Verify(p.ClientKey, bytes, sig) {
		return errors.New("companion signature rejected")
	}
	finished, ok := r["finishedAt"].(int64)
	if !ok || finished > now+100 || finished < receipt.Record["issuedAt"].(int64)-100 {
		return errors.New("receipt time mismatch")
	}
	lifecycle, lok := r["lifecycle"].(string)
	errorCode, eok := r["errorCode"].(string)
	if !lok || !eok {
		return errors.New("result lifecycle/error type")
	}
	for _, k := range []string{"viewportDigest", "targetDigest", "dataDigest"} {
		d, ok := r[k].(string)
		if !ok || !digestID.MatchString(d) {
			return errors.New("result digest type")
		}
	}
	var value any
	switch lifecycle {
	case "completed":
		if errorCode != "" {
			return errors.New("completed receipt has error")
		}
		operation := receipt.Record["operation"].(string)
		if operation == "observe" {
			if !exactFields(data, []string{"operation", "observation", "observationDigest"}) || data["operation"] != "observe" {
				return errors.New("invalid observe result")
			}
			oid, valid := data["observation"].(string)
			if !valid {
				return errors.New("observation ID type")
			}
			proof, ok := a.observations[oid]
			if !ok || proof["pair"] != p.ID || proof["digest"] != data["observationDigest"] || proof["commandId"] != id {
				return errors.New("signed observation proof missing")
			}
			nullTarget, _ := Digest("target", nil)
			if r["viewportDigest"] != proof["viewportDigest"] || r["targetDigest"] != nullTarget {
				return errors.New("observe terminal geometry mismatch")
			}
			value = data
			break
		}
		if !exactFields(data, []string{"operation", "dispatched"}) || data["operation"] != operation || data["dispatched"] != true {
			return errors.New("invalid typed result")
		}
		value = data
	case "rejected", "uncertain":
		if data != nil {
			return errors.New("noncompleted data must be null")
		}
		allowed := map[string]bool{"denied": true, "stale": true, "expired": true, "detached": true, "timeout": true, "transport": true, "driver": true, "storage": true, "signature": true}
		if !allowed[errorCode] {
			return errors.New("invalid error code")
		}
	default:
		return errors.New("unknown result lifecycle")
	}
	digest, e := Digest("result-data", value)
	if e != nil || digest != r["dataDigest"] {
		return errors.New("result payload mismatch")
	}
	if lifecycle == "completed" && receipt.Record["operation"] != "observe" {
		proof := receipt.Proof
		ok := proof != nil
		if !ok || proof["pair"] != p.ID || proof["viewportDigest"] != r["viewportDigest"] {
			return errors.New("receipt viewport mismatch")
		}
		var target any
		if receipt.Record["operation"] == "click" {
			for _, v := range proof["targets"].([]any) {
				t := v.(map[string]any)
				if t["id"] == receipt.Args["target"] {
					target = t
				}
			}
			if target == nil {
				return errors.New("receipt target absent")
			}
		}
		td, e := Digest("target", target)
		if e != nil || td != r["targetDigest"] {
			return errors.New("receipt target mismatch")
		}
		delete(a.observations, receipt.Record["observation"].(string))
	}
	if lifecycle != "completed" && receipt.Observation != "" {
		delete(a.observations, receipt.Observation)
	}
	if lifecycle == "uncertain" {
		a.detachPair(p.ID)
	}
	receipt.State = lifecycle
	a.receipts[id] = receipt
	if e = a.persist(); e != nil {
		receipt.State = "uncertain"
		a.receipts[id] = receipt
		a.detachPair(p.ID)
		return e
	}
	if lifecycle == "completed" && receipt.Record["operation"] == "observe" {
		if proof := a.observations[receipt.Observation]; proof != nil {
			proof["ready"] = true
		}
	}
	return nil
}

func (a *Authority) detachPair(pair string) {
	delete(a.pairs, pair)
	for id, p := range a.observations {
		if p["pair"] == pair {
			delete(a.observations, id)
		}
	}
}
