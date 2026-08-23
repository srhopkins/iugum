package job

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

func TestHTTPPostEmptyURL(t *testing.T) {
	fn := HTTPPost("")
	err := fn(context.Background(), contract.Event{Name: "ping"})
	if err == nil || !strings.Contains(err.Error(), "empty url") {
		t.Fatalf("want empty url error, got %v", err)
	}
}

func TestHTTPPostBodyHasEventName(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ev := contract.Event{
		Name:   "watch.changed",
		Source: "hook",
		Path:   "/tmp/x",
		Attrs:  map[string]string{"k": "v"},
	}
	fn := HTTPPost(srv.URL)
	if err := fn(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	var payload httpEventBody
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event != ev.Name {
		t.Fatalf("event = %q, want %q", payload.Event, ev.Name)
	}
	if payload.Source != ev.Source {
		t.Fatalf("source = %q, want %q", payload.Source, ev.Source)
	}
	if payload.Path != ev.Path {
		t.Fatalf("path = %q, want %q", payload.Path, ev.Path)
	}
	if payload.Attrs["k"] != "v" {
		t.Fatalf("attrs = %v, want k=v", payload.Attrs)
	}
}

func TestHTTPPostRespectsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	fn := HTTPPost(srv.URL)
	err := fn(ctx, contract.Event{Name: "slow"})
	if err == nil {
		t.Fatal("want context error")
	}
}

func TestHTTPPostClientTimeout(t *testing.T) {
	old := httpJobTimeout
	httpJobTimeout = 30 * time.Millisecond
	t.Cleanup(func() { httpJobTimeout = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fn := HTTPPost(srv.URL)
	err := fn(context.Background(), contract.Event{Name: "stall"})
	if err == nil {
		t.Fatal("want client timeout")
	}
}

func TestHTTPPostNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	fn := HTTPPost(srv.URL)
	err := fn(context.Background(), contract.Event{Name: "fail"})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("want 400 error, got %v", err)
	}
}
