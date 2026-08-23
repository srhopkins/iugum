package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

func TestE2EHTTPJob(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "notify", Kind: "http", URL: srv.URL},
	}
	a := newE2EApp(t, cfg)

	if err := a.FireHook(context.Background(), contract.Event{
		Name:   "notify",
		Source: "hook",
		Path:   "/tmp/x",
	}); err != nil {
		t.Fatal(err)
	}

	var payload struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event != "notify" {
		t.Fatalf("event = %q, want notify", payload.Event)
	}
}
