package sqladapt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPIngestThenQuery(t *testing.T) {
	s := openTest(t)
	h := Handler(s)

	body := []byte(`[{"name":"junction_c","value":81.2,"labels":{"gpu":"mi50"}},{"name":"fan_pct","value":40}]`)
	req := httptest.NewRequest(http.MethodPost, "/ingest/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}

	prom := httptest.NewRequest(http.MethodPost, "/ingest/metrics", bytes.NewReader([]byte("edge_c 70.5\n")))
	prom.Header.Set("Content-Type", "text/plain")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, prom)
	if rr.Code != http.StatusOK {
		t.Fatalf("prom ingest %d %s", rr.Code, rr.Body.String())
	}

	logs := httptest.NewRequest(http.MethodPost, "/ingest/logs", bytes.NewReader([]byte(`[{"stream":"homelab","message":"junction 81"}]`)))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, logs)
	if rr.Code != http.StatusOK {
		t.Fatalf("logs ingest %d %s", rr.Code, rr.Body.String())
	}

	q := httptest.NewRequest(http.MethodGet, `/query/metrics?q=junction_c{gpu="mi50"}`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, q)
	if rr.Code != http.StatusOK {
		t.Fatalf("query %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Series []struct {
			Name   string       `json:"Name"`
			Points [][2]float64 `json:"Points"`
		} `json:"series"`
		Marks []float64 `json:"marks"`
		Unit  string    `json:"unit"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Series) != 1 {
		t.Fatalf("series %+v", resp.Series)
	}
	if resp.Series[0].Points[0][1] != 81.2 {
		t.Fatalf("value %v", resp.Series[0].Points[0][1])
	}
	if len(resp.Marks) != 3 || resp.Marks[0] != 50 || resp.Marks[2] != 105 {
		t.Fatalf("marks %+v", resp.Marks)
	}
	if resp.Unit != "C" {
		t.Fatalf("unit %q", resp.Unit)
	}

	lq := httptest.NewRequest(http.MethodGet, `/query/logs?q={stream="homelab"}%20|=%20"junction"`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, lq)
	if rr.Code != http.StatusOK {
		t.Fatalf("log query %d %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("junction 81")) {
		t.Fatalf("logs %s", rr.Body.String())
	}
}

func TestUIEmbedded(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("iugum observe")) {
		t.Fatalf("ui %s", rr.Body.String())
	}
}

func TestMetaMarks(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	req := httptest.NewRequest(http.MethodGet, "/meta.json", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("50")) || !bytes.Contains(rr.Body.Bytes(), []byte("105")) {
		t.Fatalf("meta %s", rr.Body.String())
	}
}
