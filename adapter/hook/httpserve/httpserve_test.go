package httpserve

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestGoodHMACFiresHook(t *testing.T) {
	const secret = "test-secret"
	var fired atomic.Bool
	var got contract.Event
	h := Handler(secret, func(_ context.Context, ev contract.Event) error {
		got = ev
		fired.Store(true)
		return nil
	})

	body := []byte(`{"delivery_id":"d1","ts":"2026-08-23T10:00:00Z","data":{"k":"v"}}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/watch.changed", bytes.NewReader(body))
	req.Header.Set(signatureHeader, SignBody(secret, body))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d body %q", rr.Code, rr.Body.String())
	}
	if !fired.Load() {
		t.Fatal("hook not fired")
	}
	if got.Name != "watch.changed" {
		t.Fatalf("name %q", got.Name)
	}
	if got.Source != "http" {
		t.Fatalf("source %q", got.Source)
	}
	if got.Attrs["delivery_id"] != "d1" || got.Attrs["k"] != "v" {
		t.Fatalf("attrs %v", got.Attrs)
	}
}

func TestBadHMAC401(t *testing.T) {
	h := Handler("secret", func(context.Context, contract.Event) error {
		t.Fatal("should not fire")
		return nil
	})

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/x", bytes.NewReader(body))
	req.Header.Set(signatureHeader, "sha256=deadbeef")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestUnsignedWithSecret401(t *testing.T) {
	h := Handler("secret", func(context.Context, contract.Event) error {
		t.Fatal("should not fire")
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/hooks/x", bytes.NewReader([]byte(`{}`)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestUnsignedNoSecretFires(t *testing.T) {
	var fired atomic.Bool
	h := Handler("", func(context.Context, contract.Event) error {
		fired.Store(true)
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/hooks/dev.tick", bytes.NewReader([]byte(`{"event":"body.name"}`)))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d", rr.Code)
	}
	if !fired.Load() {
		t.Fatal("hook not fired")
	}
}

func TestWrongMethod405(t *testing.T) {
	h := Handler("", func(context.Context, contract.Event) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/hooks/x", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestUnknownPath404(t *testing.T) {
	h := Handler("", func(context.Context, contract.Event) error { return nil })
	req := httptest.NewRequest(http.MethodPost, "/nope", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d", rr.Code)
	}
}
