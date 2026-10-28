package browsergrant

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/png"
	"reflect"
)

var observationFields = append(append([]string{}, common...), "capturedAt", "expires", "viewportDigest", "targetsDigest", "dataDigest", "mediaType", "byteLength", "lifecycle")

func (a *Authority) AcceptObservation(ctx context.Context, e Envelope, viewport map[string]any, targets []any, raster []byte) error {
	return a.acceptObservation(ctx, "", "", e, viewport, targets, raster)
}
func (a *Authority) AcceptObservationForWindow(ctx context.Context, instance, generation string, e Envelope, viewport map[string]any, targets []any, raster []byte) error {
	return a.acceptObservation(ctx, instance, generation, e, viewport, targets, raster)
}
func (a *Authority) acceptObservation(ctx context.Context, instance, generation string, e Envelope, viewport map[string]any, targets []any, raster []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("authority closed")
	}
	r := e.Record
	if !exactFields(r, observationFields) {
		return errors.New("unexpected observation schema")
	}
	id, _ := r["commandId"].(string)
	receipt, ok := a.receipts[id]
	if !ok || receipt.State != "issued" || receipt.Record["operation"] != "observe" || receipt.Observation != "" {
		return errors.New("no issued observation grant")
	}
	p, ok := a.pairs[receipt.Pair]
	if !ok || instance != "" && (p.Instance != instance || p.WindowLease != generation) {
		return errors.New("unpaired observation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, k := range common {
		if k == "observation" {
			continue
		}
		if k == "grantDigest" {
			if r[k] != receipt.GrantDigest {
				return errors.New("grant swapped")
			}
		} else if !reflect.DeepEqual(r[k], receipt.Record[k]) {
			return errors.New("observation binding swapped")
		}
	}
	b, err := SignedBytes("observation", r)
	if err != nil {
		return err
	}
	sig, err := unbase64(e.Signature, 64)
	if err != nil || len(sig) != 64 || !ed25519.Verify(p.ClientKey, b, sig) {
		return errors.New("observation signature rejected")
	}
	now := a.now().UnixMilli()
	captured, ok := r["capturedAt"].(int64)
	if !ok || captured > now+100 || captured < receipt.Record["issuedAt"].(int64)-100 || captured > receipt.Record["expires"].(int64)+100 {
		return errors.New("capture time invalid")
	}
	expires, ok := r["expires"].(int64)
	if !ok || expires-100 <= now || expires > captured+5000 || expires > p.Expires {
		return errors.New("observation expired")
	}
	oid, ok := r["observation"].(string)
	if !ok || !protocolID.MatchString(oid) {
		return errors.New("observation ID required")
	}
	if _, exists := a.observations[oid]; exists {
		return errors.New("observation replay")
	}
	if r["mediaType"] != "image/png" || r["lifecycle"] != "completed" || r["byteLength"] != int64(len(raster)) || len(raster) > MaxRasterBytes {
		return errors.New("unsupported raster")
	}
	if err := strictPNG(raster); err != nil {
		return err
	}
	config, err := png.DecodeConfig(bytes.NewReader(raster))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > MaxRasterPixels {
		return errors.New("unsafe raster geometry")
	}
	if _, err = png.Decode(bytes.NewReader(raster)); err != nil {
		return errors.New("invalid PNG")
	}
	sum := sha256.Sum256(append([]byte("iugum/browser/raster/2\n"), raster...))
	if hex.EncodeToString(sum[:]) != r["dataDigest"] {
		return errors.New("raster digest mismatch")
	}
	if !exactFields(viewport, []string{"width", "height", "scrollX", "scrollY", "zoom", "scale"}) {
		return errors.New("viewport schema")
	}
	for k, v := range viewport {
		n, ok := v.(float64)
		if !ok {
			return errors.New("viewport number type")
		}
		if _, err = Canonical(n); err != nil {
			return err
		}
		if (k == "width" || k == "height" || k == "zoom" || k == "scale") && n <= 0 {
			return errors.New("viewport bounds")
		}
	}
	width, height := viewport["width"].(float64), viewport["height"].(float64)
	scale, zoom := viewport["scale"].(float64), viewport["zoom"].(float64)
	if width > 8192 || height > 8192 || scale < 0.25 || scale > 8 || zoom > 8 || zoom < 0.25 || viewport["scrollX"].(float64) < 0 || viewport["scrollY"].(float64) < 0 || width*scale != float64(config.Width) || height*scale != float64(config.Height) {
		return errors.New("raster and CSS viewport transform mismatch")
	}
	vd, err := Digest("viewport", viewport)
	if err != nil || vd != r["viewportDigest"] {
		return errors.New("viewport digest mismatch")
	}
	if len(targets) > 128 {
		return errors.New("too many targets")
	}
	seen := map[string]bool{}
	for _, v := range targets {
		target, ok := v.(map[string]any)
		if !ok || !exactFields(target, []string{"id", "bounds", "topFrame"}) || target["topFrame"] != true {
			return errors.New("target schema")
		}
		tid, ok := target["id"].(string)
		if !ok || !targetID.MatchString(tid) || seen[tid] {
			return errors.New("target identity")
		}
		seen[tid] = true
		bounds, ok := target["bounds"].(map[string]any)
		if !ok || !exactFields(bounds, []string{"x", "y", "width", "height"}) {
			return errors.New("target bounds")
		}
		for k, v := range bounds {
			n, ok := v.(float64)
			if !ok {
				return errors.New("target number")
			}
			if _, err = Canonical(n); err != nil {
				return err
			}
			if (k == "width" || k == "height") && n <= 0 {
				return errors.New("empty target")
			}
		}
		if bounds["x"].(float64) < 0 || bounds["y"].(float64) < 0 || bounds["x"].(float64)+bounds["width"].(float64) > width || bounds["y"].(float64)+bounds["height"].(float64) > height {
			return errors.New("target outside viewport")
		}
	}
	td, err := Digest("targets", targets)
	if err != nil || td != r["targetsDigest"] {
		return errors.New("targets digest mismatch")
	}
	digest, err := Digest("observation", r)
	if err != nil {
		return err
	}
	// Clone via typed canonical-compatible tree; caller cannot mutate saved proof.
	receipt.Observation = oid
	a.receipts[id] = receipt
	if err = a.persist(); err != nil {
		receipt.State = "uncertain"
		a.receipts[id] = receipt
		delete(a.pairs, p.ID)
		return err
	}
	a.observations[oid] = map[string]any{"commandId": id, "ready": false, "pair": p.ID, "expires": expires - 100, "viewportDigest": vd, "targets": cloneArray(targets), "digest": digest}
	return nil
}
func cloneArray(v []any) []any {
	out := make([]any, len(v))
	for i, x := range v {
		if m, ok := x.(map[string]any); ok {
			out[i] = cloneMap(m)
		} else {
			out[i] = x
		}
	}
	return out
}
func cloneMap(v map[string]any) map[string]any {
	out := map[string]any{}
	for k, x := range v {
		if m, ok := x.(map[string]any); ok {
			out[k] = cloneMap(m)
		} else {
			out[k] = x
		}
	}
	return out
}
