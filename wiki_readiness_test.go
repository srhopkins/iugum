package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWikiReadinessWaitsForPage(t *testing.T) {
	var ready atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer s.Close()
	done := make(chan error, 1)
	go func() { done <- waitWikiReady(context.Background(), s.URL) }()
	select {
	case err := <-done:
		t.Fatalf("reported readiness before page: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	ready.Store(true)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ready wiki not detected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitWikiReady(ctx, s.URL) == nil {
		t.Fatal("ignored cancellation")
	}
}
