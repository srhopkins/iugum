package browsergrant

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/srhopkins/iugum/policy"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func observationFixture(t *testing.T) (*Authority, ed25519.PrivateKey, Envelope, map[string]any, []any, []byte) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "policy.csv")
	os.WriteFile(p, []byte("p, tester, agent/browser/*, write, allow\n"), 0600)
	g, e := policy.New("", p)
	if e != nil {
		t.Fatal(e)
	}
	a, e := New(filepath.Join(t.TempDir(), "receipts.json"), "tester", "incarnation", g)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	a.RegisterTrustedPair(Pair{ID: "pair", Authority: "incarnation", ServerKeyID: "server", ClientKeyID: "clientkey", Instance: "instance", WindowLease: "lease", Client: "client", Document: "document", Origin: "https://iugum-agent-code.local", TabID: 17, Expires: time.Now().Add(time.Minute).UnixMilli(), ClientKey: pub})
	grant, e := a.Grant(context.Background(), Request{Pair: "pair", CommandID: newID(), Operation: "observe", Args: map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	viewport := map[string]any{"width": float64(1), "height": float64(1), "scrollX": float64(0), "scrollY": float64(0), "zoom": float64(1), "scale": float64(1)}
	targets := []any{map[string]any{"id": "button", "topFrame": true, "bounds": map[string]any{"x": float64(0), "y": float64(0), "width": float64(1), "height": float64(1)}}}
	var out bytes.Buffer
	png.Encode(&out, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	r := map[string]any{}
	for _, k := range common {
		r[k] = grant.Record[k]
	}
	r["grantDigest"], _ = Digest("grant", grant.Record)
	r["observation"] = newID()
	r["capturedAt"] = a.now().UnixMilli()
	r["expires"] = a.now().Add(5 * time.Second).UnixMilli()
	r["viewportDigest"], _ = Digest("viewport", viewport)
	r["targetsDigest"], _ = Digest("targets", targets)
	sum := sha256.Sum256(append([]byte("iugum/browser/raster/2\n"), out.Bytes()...))
	r["dataDigest"] = hex.EncodeToString(sum[:])
	r["mediaType"] = "image/png"
	r["byteLength"] = int64(out.Len())
	r["lifecycle"] = "completed"
	return a, key, signEnvelope(t, key, "observation", r), viewport, targets, out.Bytes()
}
func signEnvelope(t *testing.T, key ed25519.PrivateKey, domain string, r map[string]any) Envelope {
	t.Helper()
	b, e := SignedBytes(domain, r)
	if e != nil {
		t.Fatal(e)
	}
	return Envelope{r, base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b))}
}
func TestSignedObservationAndTypedResultOneUse(t *testing.T) {
	a, key, e, v, targets, png := observationFixture(t)
	ctx := context.Background()
	if err := a.AcceptObservation(ctx, e, v, targets, png); err != nil {
		t.Fatal(err)
	}
	if err := a.AcceptObservation(ctx, e, v, targets, png); err == nil {
		t.Fatal("accepted observation twice")
	}
	// A captured proof remains unavailable until the signed completed observe receipt.
	original := e.Record["commandId"].(string)
	terminal := map[string]any{}
	for _, k := range common {
		terminal[k] = e.Record[k]
	}
	terminal["observation"] = ""
	terminal["viewportDigest"] = e.Record["viewportDigest"]
	terminal["targetDigest"], _ = Digest("target", nil)
	observedData := map[string]any{"operation": "observe", "observation": e.Record["observation"], "observationDigest": a.observations[e.Record["observation"].(string)]["digest"]}
	terminal["dataDigest"], _ = Digest("result-data", observedData)
	terminal["lifecycle"] = "completed"
	terminal["finishedAt"] = a.now().UnixMilli()
	terminal["errorCode"] = ""
	if a.receipts[original].Observation == "" {
		t.Fatal("missing stored observation")
	}
	if err := a.AcceptResult(ctx, signEnvelope(t, key, "result", terminal), observedData); err != nil {
		t.Fatal(err)
	}
	oid := e.Record["observation"].(string)
	q := Request{Pair: "pair", CommandID: newID(), Operation: "click", Observation: oid, Args: map[string]any{"x": float64(.5), "y": float64(.5), "target": "button"}}
	grant, err := a.Grant(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	q.CommandID = newID()
	if _, err = a.Grant(ctx, q); err == nil {
		t.Fatal("reused old observation under new command")
	}
	r := map[string]any{}
	for _, k := range common {
		r[k] = grant.Record[k]
	}
	r["grantDigest"], _ = Digest("grant", grant.Record)
	r["viewportDigest"] = e.Record["viewportDigest"]
	r["targetDigest"], _ = Digest("target", targets[0])
	data := map[string]any{"operation": "click", "dispatched": true}
	r["dataDigest"], _ = Digest("result-data", data)
	r["lifecycle"] = "completed"
	r["finishedAt"] = a.now().UnixMilli()
	r["errorCode"] = ""
	receipt := signEnvelope(t, key, "result", r)
	if err = a.AcceptResult(ctx, receipt, data); err != nil {
		t.Fatal(err)
	}
	if err = a.AcceptResult(ctx, receipt, data); err == nil {
		t.Fatal("terminal receipt replay")
	}
}
func TestObservationRejectsCorruptionAndActiveFormats(t *testing.T) {
	for _, kind := range []string{"signature", "raster", "viewport", "targets", "identity"} {
		t.Run(kind, func(t *testing.T) {
			a, key, e, v, targets, png := observationFixture(t)
			switch kind {
			case "signature":
				e.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
			case "raster":
				png = []byte("<svg onload=evil />")
			case "viewport":
				v["scrollX"] = float64(1)
			case "targets":
				targets[0].(map[string]any)["id"] = "swapped"
			case "identity":
				e.Record["document"] = "other"
				e = signEnvelope(t, key, "observation", e.Record)
			}
			if err := a.AcceptObservation(context.Background(), e, v, targets, png); err == nil {
				t.Fatal("accepted corruption")
			}
		})
	}
}
func observeTerminal(t *testing.T, a *Authority, key ed25519.PrivateKey, e Envelope, lifecycle string) (Envelope, map[string]any) {
	r := map[string]any{}
	for _, k := range common {
		r[k] = e.Record[k]
	}
	r["observation"] = ""
	r["viewportDigest"] = e.Record["viewportDigest"]
	r["targetDigest"], _ = Digest("target", nil)
	r["lifecycle"] = lifecycle
	r["finishedAt"] = a.now().UnixMilli()
	r["errorCode"] = ""
	var data map[string]any
	if lifecycle == "completed" {
		oid := e.Record["observation"].(string)
		data = map[string]any{"operation": "observe", "observation": oid, "observationDigest": a.observations[oid]["digest"]}
	} else {
		r["errorCode"] = "driver"
	}
	var payload any
	if data != nil {
		payload = data
	}
	r["dataDigest"], _ = Digest("result-data", payload)
	return signEnvelope(t, key, "result", r), data
}
func TestObservationRequiresExactCompletedTerminalBeforeClick(t *testing.T) {
	for _, mode := range []string{"pending", "wrong-viewport", "wrong-target", "rejected", "uncertain", "completed"} {
		t.Run(mode, func(t *testing.T) {
			a, key, ob, v, targets, png := observationFixture(t)
			ctx := context.Background()
			if err := a.AcceptObservation(ctx, ob, v, targets, png); err != nil {
				t.Fatal(err)
			}
			state := "completed"
			if mode == "rejected" || mode == "uncertain" {
				state = mode
			}
			terminal, data := observeTerminal(t, a, key, ob, state)
			switch mode {
			case "wrong-viewport":
				terminal.Record["viewportDigest"] = "0"
				terminal = signEnvelope(t, key, "result", terminal.Record)
			case "wrong-target":
				terminal.Record["targetDigest"] = "0"
				terminal = signEnvelope(t, key, "result", terminal.Record)
			}
			if mode != "pending" {
				err := a.AcceptResult(ctx, terminal, data)
				if mode == "wrong-viewport" || mode == "wrong-target" {
					if err == nil {
						t.Fatal("wrong geometry terminal accepted")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
			_, err := a.Grant(ctx, Request{Pair: "pair", CommandID: newID(), Operation: "click", Observation: ob.Record["observation"].(string), Args: map[string]any{"x": float64(.5), "y": float64(.5), "target": "button"}})
			if mode == "completed" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("incomplete/failed observation enabled click")
			}
			if mode == "rejected" || mode == "uncertain" {
				if _, ok := a.observations[ob.Record["observation"].(string)]; ok {
					t.Fatal("failed capture proof retained")
				}
			}
		})
	}
}
func TestRasterViewportTransformAndDPR2(t *testing.T) {
	for _, mode := range []string{"dpr2", "mismatch", "nonuniform", "target-outside"} {
		t.Run(mode, func(t *testing.T) {
			a, key, e, v, targets, _ := observationFixture(t)
			var imageData bytes.Buffer
			png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
			v["scale"] = float64(2)
			if mode == "mismatch" {
				v["width"] = float64(2)
			}
			if mode == "nonuniform" {
				v["height"] = float64(2)
			}
			if mode == "target-outside" {
				targets[0].(map[string]any)["bounds"].(map[string]any)["x"] = float64(1)
			}
			e.Record["viewportDigest"], _ = Digest("viewport", v)
			e.Record["targetsDigest"], _ = Digest("targets", targets)
			sum := sha256.Sum256(append([]byte("iugum/browser/raster/2\n"), imageData.Bytes()...))
			e.Record["dataDigest"] = hex.EncodeToString(sum[:])
			e.Record["byteLength"] = int64(imageData.Len())
			e = signEnvelope(t, key, "observation", e.Record)
			err := a.AcceptObservation(context.Background(), e, v, targets, imageData.Bytes())
			if mode == "dpr2" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("incoherent raster/target accepted")
			}
		})
	}
}
func TestRasterExact16MiPixelBoundary(t *testing.T) {
	a, key, e, v, targets, _ := observationFixture(t)
	// Isolate raster size boundary from CPU-heavy PNG encoding under race instrumentation.
	fixed := a.now()
	a.now = func() time.Time { return fixed }
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 4096, 4096))); err != nil {
		t.Fatal(err)
	}
	v["width"], v["height"] = float64(4096), float64(4096)
	e.Record["viewportDigest"], _ = Digest("viewport", v)
	sum := sha256.Sum256(append([]byte("iugum/browser/raster/2\n"), imageData.Bytes()...))
	e.Record["dataDigest"] = hex.EncodeToString(sum[:])
	e.Record["byteLength"] = int64(imageData.Len())
	e.Record["capturedAt"] = a.now().UnixMilli()
	e.Record["expires"] = a.now().Add(5 * time.Second).UnixMilli()
	e = signEnvelope(t, key, "observation", e.Record)
	if err := a.AcceptObservation(context.Background(), e, v, targets, imageData.Bytes()); err != nil {
		t.Fatal("shared16Mi pixel boundary rejected", err)
	}
}
func TestCaptureClockToleranceAndGrantWindow(t *testing.T) {
	for _, delta := range []int64{-101, -50, 50, 101} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			a, key, e, v, targets, png := observationFixture(t)
			fixed := a.now()
			a.now = func() time.Time { return fixed }
			receipt := a.receipts[e.Record["commandId"].(string)]
			if delta < 0 {
				e.Record["capturedAt"] = receipt.Record["issuedAt"].(int64) + delta
			} else {
				e.Record["capturedAt"] = fixed.UnixMilli() + delta
			}
			e.Record["expires"] = e.Record["capturedAt"].(int64) + 5000
			e = signEnvelope(t, key, "observation", e.Record)
			err := a.AcceptObservation(context.Background(), e, v, targets, png)
			if delta == -50 || delta == 50 {
				if err != nil {
					t.Fatal("valid bounded clock denied", err)
				}
			} else if err == nil {
				t.Fatal("out of bound clock accepted")
			}
		})
	}
	a, key, e, v, targets, png := observationFixture(t)
	receipt := a.receipts[e.Record["commandId"].(string)]
	expiry := receipt.Record["expires"].(int64)
	a.now = func() time.Time { return time.UnixMilli(expiry + 200) }
	e.Record["capturedAt"] = expiry + 101
	e.Record["expires"] = expiry + 5101
	e = signEnvelope(t, key, "observation", e.Record)
	if err := a.AcceptObservation(context.Background(), e, v, targets, png); err == nil {
		t.Fatal("expired observe grant minted fresh capture")
	}
}
func TestResultClockTolerance(t *testing.T) {
	for _, delta := range []int64{-101, -50, 50, 101} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			a, key, e, v, targets, png := observationFixture(t)
			fixed := a.now()
			a.now = func() time.Time { return fixed }
			if err := a.AcceptObservation(context.Background(), e, v, targets, png); err != nil {
				t.Fatal(err)
			}
			terminal, data := observeTerminal(t, a, key, e, "completed")
			receipt := a.receipts[e.Record["commandId"].(string)]
			if delta < 0 {
				terminal.Record["finishedAt"] = receipt.Record["issuedAt"].(int64) + delta
			} else {
				terminal.Record["finishedAt"] = fixed.UnixMilli() + delta
			}
			terminal = signEnvelope(t, key, "result", terminal.Record)
			err := a.AcceptResult(context.Background(), terminal, data)
			if delta == -50 || delta == 50 {
				if err != nil {
					t.Fatal("valid bounded result clock denied", err)
				}
			} else if err == nil {
				t.Fatal("out of bound result time accepted")
			}
		})
	}
}
