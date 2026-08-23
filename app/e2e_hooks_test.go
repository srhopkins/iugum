package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/srhopkins/iugum/adapter/hook/httpserve"
	"github.com/srhopkins/iugum/contract"
)

func TestE2EHookRegisterOnFire(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	var gotPath string
	a.Hooks.Register("echo", func(_ context.Context, ev contract.Event) error {
		gotPath = ev.Path
		return nil
	})
	a.Hooks.On("watch.changed", "echo")

	if err := a.FireHook(ctx, contract.Event{Name: "watch.changed", Path: "/tmp/x"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/tmp/x" {
		t.Fatalf("got path %q", gotPath)
	}
}

func TestE2EHookWorkflowStopsOnError(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	var order []string
	a.Hooks.Register("ok", func(context.Context, contract.Event) error {
		order = append(order, "ok")
		return nil
	})
	a.Hooks.Register("bad", func(context.Context, contract.Event) error {
		order = append(order, "bad")
		return errors.New("stop")
	})
	a.Hooks.Register("never", func(context.Context, contract.Event) error {
		order = append(order, "never")
		return nil
	})
	a.Hooks.AddWorkflow("pipe", []string{"ok", "bad", "never"})

	err := a.FireHook(ctx, contract.Event{Name: "pipe"})
	if err == nil {
		t.Fatal("want workflow error")
	}
	if len(order) != 2 || order[1] != "bad" {
		t.Fatalf("order %v", order)
	}
}

func TestE2EMemIngestBuiltinViaText(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := a.FireHook(ctx, contract.Event{
		Name:  "mem.ingest",
		Attrs: map[string]string{"text": "Steve owns the MI50."},
	}); err != nil {
		t.Fatal(err)
	}

	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "default", From: "steve", Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range walk {
		if h.Rel == "owns" && h.To == "mi50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ingest did not create edge, walk=%+v", walk)
	}
}

func TestE2EMemIngestBuiltinViaPath(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("Steve owns the MI50."), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := a.FireHook(ctx, contract.Event{Name: "mem.ingest", Path: path}); err != nil {
		t.Fatal(err)
	}

	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "default", From: "steve", Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range walk {
		if h.Rel == "owns" && h.To == "mi50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("path ingest did not create edge, walk=%+v", walk)
	}
}

func TestE2EMemIngestBuiltinMissingPathFails(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	err := a.FireHook(ctx, contract.Event{Name: "mem.ingest", Path: filepath.Join(t.TempDir(), "missing.txt")})
	if err == nil {
		t.Fatal("want read error for missing path")
	}
}

func randomHookSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestE2EHMACGoodSignatureThroughApp(t *testing.T) {
	secret := randomHookSecret(t)
	a := newE2EApp(t, e2eConfig(t))

	var fired atomic.Bool
	var got contract.Event
	a.Hooks.Register("http.echo", func(_ context.Context, ev contract.Event) error {
		got = ev
		fired.Store(true)
		return nil
	})
	a.Hooks.On("watch.changed", "http.echo")

	h := httpserve.Handler(secret, a.FireHook)

	body := []byte(`{"event":"watch.changed","delivery_id":"d-e2e","data":{"note":"hi"}}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/watch.changed", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", httpserve.SignBody(secret, body))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d body %q", rr.Code, rr.Body.String())
	}
	if !fired.Load() {
		t.Fatal("hook not fired through app")
	}
	if got.Name != "watch.changed" {
		t.Fatalf("name %q", got.Name)
	}
	if got.Attrs["delivery_id"] != "d-e2e" || got.Attrs["note"] != "hi" {
		t.Fatalf("attrs %v", got.Attrs)
	}
}

func TestE2EHMACBadSignature401(t *testing.T) {
	secret := randomHookSecret(t)
	a := newE2EApp(t, e2eConfig(t))

	a.Hooks.Register("http.echo", func(context.Context, contract.Event) error {
		t.Fatal("should not fire")
		return nil
	})

	h := httpserve.Handler(secret, a.FireHook)

	body := []byte(`{"event":"watch.changed"}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/watch.changed", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
}
