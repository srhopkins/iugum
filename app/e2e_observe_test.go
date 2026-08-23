package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/srhopkins/iugum/adapter/observe/sqladapt"
	"github.com/srhopkins/iugum/contract"
)

func observeE2EApp(t *testing.T) *App {
	t.Helper()
	cfg := e2eConfig(t)
	cfg.Observe = "sqlite"
	return newE2EApp(t, cfg)
}

func TestE2EObserveIngestQueryLogsHTTP(t *testing.T) {
	a := observeE2EApp(t)
	ctx := context.Background()
	now := int64(1_700_000_000_000_000)

	samples := []contract.Sample{
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50"}, Value: 81.4, TimeUS: now},
		{Name: "edge_c", Labels: map[string]string{"gpu": "mi50"}, Value: 72.0, TimeUS: now},
		{Name: "mem_c", Labels: map[string]string{"gpu": "mi50"}, Value: 68.0, TimeUS: now},
		{Name: "cpu_c", Labels: map[string]string{"host": "homelab"}, Value: 46.0, TimeUS: now},
		{Name: "fan_pct", Labels: map[string]string{"header": "SYS_FAN1"}, Value: 40.0, TimeUS: now},
	}
	if err := a.IngestMetrics(ctx, samples); err != nil {
		t.Fatal(err)
	}
	series, err := a.QueryMetrics(ctx, contract.MetricQuery{Name: "junction_c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Name != "junction_c" {
		t.Fatalf("metrics %+v", series)
	}
	if series[0].Points[0][1] > 200 {
		t.Fatalf("millidegrees leaked: %v", series[0].Points[0][1])
	}

	if err := a.IngestLogs(ctx, []contract.Log{
		{Stream: "homelab", Message: "MI50 junction 81 fan 40"},
	}); err != nil {
		t.Fatal(err)
	}
	logs, err := a.SearchLogs(ctx, contract.LogQuery{Expr: `{stream="homelab"} |= "junction"`})
	if err != nil || len(logs) != 1 {
		t.Fatalf("logs %+v err=%v", logs, err)
	}

	h := a.ObserveHandler()
	body := []byte(`[{"name":"junction_c","value":82.0,"labels":{"gpu":"mi50"}}]`)
	req := httptest.NewRequest(http.MethodPost, "/ingest/metrics", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("http ingest %d %s", rr.Code, rr.Body.String())
	}

	q := httptest.NewRequest(http.MethodGet, `/query/metrics?q=junction_c`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, q)
	if rr.Code != http.StatusOK {
		t.Fatalf("http query %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Series []contract.Series `json:"series"`
		Marks  []float64         `json:"marks"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Series) == 0 {
		t.Fatal("no series")
	}
	found := false
	for _, s := range resp.Series {
		if s.Name == "junction_c" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing junction_c in %+v", resp.Series)
	}
	if len(resp.Marks) != 3 || resp.Marks[0] != 50 || resp.Marks[1] != 100 || resp.Marks[2] != 105 {
		t.Fatalf("marks %+v want 50/100/105", resp.Marks)
	}

	meta := httptest.NewRequest(http.MethodGet, "/meta.json", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, meta)
	if !bytes.Contains(rr.Body.Bytes(), []byte("junction_c")) || !bytes.Contains(rr.Body.Bytes(), []byte("SYS_FAN1")) {
		t.Fatalf("meta %s", rr.Body.String())
	}
	if len(sqladapt.TempMarksC) != 3 {
		t.Fatalf("mark-line metadata missing")
	}
}

func TestServeObserveStopsOnCancel(t *testing.T) {
	a := observeE2EApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.ServeObserve(ctx, port, "127.0.0.1") }()
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeObserve did not return after cancel")
	}
}
