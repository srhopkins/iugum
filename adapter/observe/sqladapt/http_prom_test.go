package sqladapt

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

const promSampleUS = int64(1_756_003_600_000_000) // 1756003600 s

func ingestJunction(t *testing.T, s *Store) {
	t.Helper()
	err := s.IngestMetrics(context.Background(), []contract.Sample{
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50"}, Value: 81.2, TimeUS: promSampleUS - 15_000_000},
		{Name: "junction_c", Labels: map[string]string{"gpu": "mi50"}, Value: 82.5, TimeUS: promSampleUS},
		{Name: "edge_c", Labels: map[string]string{"gpu": "vii"}, Value: 70, TimeUS: promSampleUS},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestPromHealthy(t *testing.T) {
	h := Handler(openTest(t))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/-/healthy", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestPromQueryInstant(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	rr := postForm(Handler(s), "/api/v1/query", url.Values{
		"query": {`junction_c{gpu="mi50"}`},
		"time":  {"1756003600"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || resp.Data.ResultType != "vector" || len(resp.Data.Result) != 1 {
		t.Fatalf("resp %+v", resp)
	}
	if resp.Data.Result[0].Metric["__name__"] != "junction_c" || resp.Data.Result[0].Metric["gpu"] != "mi50" {
		t.Fatalf("metric %+v", resp.Data.Result[0].Metric)
	}
	if resp.Data.Result[0].Value[1] != "82.5" {
		t.Fatalf("value %+v", resp.Data.Result[0].Value)
	}
}

func TestPromQueryRange(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	rr := postForm(Handler(s), "/api/v1/query_range", url.Values{
		"query": {`junction_c{gpu="mi50"}`},
		"start": {"1756003585"},
		"end":   {"1756003600"},
		"step":  {"15"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]any           `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || resp.Data.ResultType != "matrix" || len(resp.Data.Result) != 1 {
		t.Fatalf("resp %+v", resp)
	}
	if len(resp.Data.Result[0].Values) != 2 || resp.Data.Result[0].Values[1][1] != "82.5" {
		t.Fatalf("values %+v", resp.Data.Result[0].Values)
	}
}

func TestPromLabelsAndValues(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	h := Handler(s)

	rr := postForm(h, "/api/v1/labels", url.Values{})
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
	if labels.Status != "success" || !containsStr(labels.Data, "__name__") || !containsStr(labels.Data, "gpu") {
		t.Fatalf("labels %+v", labels.Data)
	}

	req := httptest.NewRequest(http.MethodGet, `/api/v1/label/gpu/values`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("values %d %s", rr.Code, rr.Body.String())
	}
	var vals struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if !containsStr(vals.Data, "mi50") || !containsStr(vals.Data, "vii") {
		t.Fatalf("gpu values %+v", vals.Data)
	}

	req = httptest.NewRequest(http.MethodGet, `/api/v1/label/__name__/values`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if err := json.Unmarshal(rr.Body.Bytes(), &vals); err != nil {
		t.Fatal(err)
	}
	if !containsStr(vals.Data, "junction_c") || !containsStr(vals.Data, "edge_c") {
		t.Fatalf("name values %+v", vals.Data)
	}
}

func TestPromMetadata(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	rr := httptest.NewRecorder()
	Handler(s).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/metadata?metric=junction_c", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   map[string][]struct {
			Type string `json:"type"`
			Help string `json:"help"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || len(resp.Data["junction_c"]) != 1 || resp.Data["junction_c"][0].Type != "gauge" {
		t.Fatalf("meta %+v", resp)
	}
}

func TestPromSeries(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	rr := postForm(Handler(s), "/api/v1/series", url.Values{
		"match[]": {`junction_c{gpu="mi50"}`},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || len(resp.Data) != 1 {
		t.Fatalf("resp %+v", resp)
	}
	if resp.Data[0]["__name__"] != "junction_c" || resp.Data[0]["gpu"] != "mi50" {
		t.Fatalf("series %+v", resp.Data[0])
	}
}

func TestPromParseQuery(t *testing.T) {
	s := openTest(t)
	rr := postForm(Handler(s), "/api/v1/parse_query", url.Values{
		"query": {`junction_c{gpu="mi50"}`},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			Matchers []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"matchers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "success" || resp.Data.Type != "vectorSelector" || resp.Data.Name != "junction_c" {
		t.Fatalf("ast %+v", resp)
	}
	if len(resp.Data.Matchers) < 2 {
		t.Fatalf("matchers %+v", resp.Data.Matchers)
	}

	rr = postForm(Handler(s), "/api/v1/parse_query", url.Values{
		"query": {`rate(junction_c[20s])`},
	})
	var call struct {
		Data struct {
			Type string `json:"type"`
			Func struct {
				Name string `json:"name"`
			} `json:"func"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &call); err != nil {
		t.Fatal(err)
	}
	if call.Data.Type != "call" || call.Data.Func.Name != "rate" {
		t.Fatalf("rate ast %s", rr.Body.String())
	}
}

func TestPromQueryBadExpr(t *testing.T) {
	rr := postForm(Handler(openTest(t)), "/api/v1/query", url.Values{"query": {"rate(junction_c)"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
	var resp struct {
		Status    string `json:"status"`
		ErrorType string `json:"errorType"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "error" || resp.ErrorType != "bad_data" {
		t.Fatalf("err %+v", resp)
	}
}

func TestPromKeepsQueryMetrics(t *testing.T) {
	s := openTest(t)
	ingestJunction(t, s)
	h := Handler(s)
	body := []byte(`[{"name":"junction_c","value":90,"labels":{"gpu":"mi50"}}]`)
	req := httptest.NewRequest(http.MethodPost, "/ingest/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ingest %d %s", rr.Code, rr.Body.String())
	}
	q := httptest.NewRequest(http.MethodGet, `/query/metrics?q=junction_c{gpu="mi50"}`, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, q)
	if rr.Code != http.StatusOK || !bytes.Contains(rr.Body.Bytes(), []byte("junction_c")) {
		t.Fatalf("query/metrics %d %s", rr.Code, rr.Body.String())
	}
}

func containsStr(in []string, want string) bool {
	for _, s := range in {
		if s == want {
			return true
		}
	}
	return false
}
