package sqladapt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func ingestLogs(t *testing.T, h http.Handler, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ingest/logs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}
}

func lokiGET(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestLokiQueryRangeStreams(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	if err := s.IngestLogs(t.Context(), []contract.Log{
		{Stream: "homelab", Message: "junction 81", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "later", TimeUS: 9_000_000},
		{Stream: "other", Message: "skip", TimeUS: 2_500_000},
	}); err != nil {
		t.Fatal(err)
	}
	ingestLogs(t, h, `[{"stream":"homelab","message":"via http"}]`)

	q := url.QueryEscape(`{stream="homelab"} |= "junction"`)
	rr := lokiGET(t, h, "/loki/api/v1/query_range?query="+q+"&start=1&end=4&limit=200&direction=backward")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || resp.Data.ResultType != "streams" {
		t.Fatalf("envelope %+v", resp)
	}
	if len(resp.Data.Result) != 1 {
		t.Fatalf("streams %+v", resp.Data.Result)
	}
	if resp.Data.Result[0].Stream["stream"] != "homelab" {
		t.Fatalf("labels %+v", resp.Data.Result[0].Stream)
	}
	if len(resp.Data.Result[0].Values) != 1 || resp.Data.Result[0].Values[0][1] != "junction 81" {
		t.Fatalf("values %+v", resp.Data.Result[0].Values)
	}
	if resp.Data.Result[0].Values[0][0] != "2000000000" {
		t.Fatalf("ns timestamp %q", resp.Data.Result[0].Values[0][0])
	}

	legacy := httptest.NewRequest(http.MethodGet, `/query/logs?q=`+q, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, legacy)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("junction 81")) {
		t.Fatalf("query/logs %d %s", rr.Code, rr.Body.String())
	}
}

func TestLokiQueryInstant(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	_ = s.IngestLogs(t.Context(), []contract.Log{
		{Stream: "homelab", Message: "fan 40", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "after", TimeUS: 9_000_000},
	})
	q := url.QueryEscape(`{stream="homelab"}`)
	rr := lokiGET(t, h, "/loki/api/v1/query?query="+q+"&time=3&limit=50")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || resp.Data.ResultType != "streams" {
		t.Fatalf("envelope %+v", resp)
	}
	if len(resp.Data.Result) != 1 || len(resp.Data.Result[0].Values) != 1 || resp.Data.Result[0].Values[0][1] != "fan 40" {
		t.Fatalf("result %+v", resp.Data.Result)
	}
}

func TestLokiQueryRangeForward(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	_ = s.IngestLogs(t.Context(), []contract.Log{
		{Stream: "homelab", Message: "one", TimeUS: 1_000_000},
		{Stream: "homelab", Message: "two", TimeUS: 2_000_000},
		{Stream: "homelab", Message: "three", TimeUS: 3_000_000},
	})
	q := url.QueryEscape(`{stream="homelab"}`)
	rr := lokiGET(t, h, "/loki/api/v1/query_range?query="+q+"&start=1&end=4&direction=forward")
	var resp struct {
		Data struct {
			Result []struct {
				Values [][2]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	vals := resp.Data.Result[0].Values
	if len(vals) != 3 || vals[0][1] != "one" || vals[2][1] != "three" {
		t.Fatalf("forward %+v", vals)
	}
}

func TestLokiLabelsAndValues(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	_ = s.IngestLogs(t.Context(), []contract.Log{
		{Stream: "homelab", Level: "error", Message: "fan stall", Attrs: map[string]string{"host": "tower"}, TimeUS: 2_000_000},
		{Stream: "wiki", Level: "info", Message: "started", TimeUS: 2_100_000},
	})
	rr := lokiGET(t, h, "/loki/api/v1/labels?start=1&end=4")
	if rr.Code != http.StatusOK {
		t.Fatalf("labels %d %s", rr.Code, rr.Body.String())
	}
	var labels struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &labels); err != nil {
		t.Fatal(err)
	}
	if labels.Status != "success" || !containsAll(labels.Data, "stream", "level", "host") {
		t.Fatalf("labels %+v", labels.Data)
	}

	rr = lokiGET(t, h, "/loki/api/v1/label/stream/values?start=1&end=4")
	var vals struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if vals.Status != "success" || !containsAll(vals.Data, "homelab", "wiki") {
		t.Fatalf("values %+v", vals.Data)
	}

	q := url.QueryEscape(`{stream="homelab"}`)
	rr = lokiGET(t, h, "/loki/api/v1/label/host/values?query="+q+"&start=1&end=4")
	if err := json.Unmarshal(rr.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if len(vals.Data) != 1 || vals.Data[0] != "tower" {
		t.Fatalf("filtered host %+v", vals.Data)
	}
}

func TestLokiSeriesVolumeStats(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	_ = s.IngestLogs(t.Context(), []contract.Log{
		{Stream: "homelab", Message: "abc", TimeUS: 2_000_000},
		{Stream: "wiki", Message: "xy", TimeUS: 2_500_000},
	})
	q := url.QueryEscape(`{stream="homelab"}`)

	rr := lokiGET(t, h, "/loki/api/v1/series?match[]="+q+"&start=1&end=4")
	var series struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &series); err != nil {
		t.Fatal(err)
	}
	if series.Status != "success" || len(series.Data) != 1 || series.Data[0]["stream"] != "homelab" {
		t.Fatalf("series %+v", series)
	}

	rr = lokiGET(t, h, "/loki/api/v1/index/volume?query="+q+"&start=1&end=4")
	var vol struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vol); err != nil {
		t.Fatal(err)
	}
	if vol.Status != "success" || vol.Data.ResultType != "vector" || len(vol.Data.Result) != 1 {
		t.Fatalf("volume %+v", vol)
	}
	if vol.Data.Result[0].Value[1] != "3" {
		t.Fatalf("volume bytes %+v", vol.Data.Result[0].Value)
	}

	rr = lokiGET(t, h, "/loki/api/v1/index/volume_range?query="+q+"&start=1&end=4&step=1")
	var vr struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Values [][]any `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vr); err != nil {
		t.Fatal(err)
	}
	if vr.Status != "success" || vr.Data.ResultType != "matrix" || len(vr.Data.Result) != 1 || len(vr.Data.Result[0].Values) == 0 {
		t.Fatalf("volume_range %+v", vr)
	}

	rr = lokiGET(t, h, "/loki/api/v1/index/stats?query="+q+"&start=1&end=4")
	var stats map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if _, ok := stats["status"]; ok {
		t.Fatalf("stats must not wrap status: %s", rr.Body.String())
	}
	if stats["streams"] != float64(1) || stats["entries"] != float64(1) || stats["bytes"] != float64(3) || stats["chunks"] != float64(1) {
		t.Fatalf("stats %+v", stats)
	}
}

func TestLokiEmptyAndBadQuery(t *testing.T) {
	s := openTest(t)
	h := Handler(s)
	q := url.QueryEscape(`{stream="missing"}`)
	rr := lokiGET(t, h, "/loki/api/v1/query_range?query="+q+"&start=1&end=4")
	var empty struct {
		Status string `json:"status"`
		Data   struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Status != "success" || empty.Data.Result == nil || len(empty.Data.Result) != 0 {
		t.Fatalf("empty %+v body=%s", empty, rr.Body.String())
	}

	rr = lokiGET(t, h, "/loki/api/v1/query_range?query=not-logql&start=1&end=4")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", rr.Code, rr.Body.String())
	}
	var errBody struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Status != "error" || errBody.Error == "" {
		t.Fatalf("error envelope %+v", errBody)
	}

	rr = lokiGET(t, h, "/loki/api/v1/index/volume?query="+q+"&start=1&end=4")
	var vol struct {
		Data struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vol); err != nil {
		t.Fatal(err)
	}
	if vol.Data.Result == nil || len(vol.Data.Result) != 0 {
		t.Fatalf("empty volume %+v", vol)
	}
}

func containsAll(have []string, want ...string) bool {
	set := map[string]struct{}{}
	for _, s := range have {
		set[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}
