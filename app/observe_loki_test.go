package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestObserveActLokiIsQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query_range?query=%7Bstream%3D%22x%22%7D", nil)
	act, ok := observeAct(req)
	if !ok || act != "query" {
		t.Fatalf("got act=%q ok=%v", act, ok)
	}
	logs := httptest.NewRequest(http.MethodGet, "/query/logs?q=%7Bstream%3D%22x%22%7D", nil)
	act, ok = observeAct(logs)
	if !ok || act != "query" {
		t.Fatalf("query/logs got act=%q ok=%v", act, ok)
	}
}
