package embedhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaRequestShape(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{0.1, 0.2}},
		})
	}))
	defer srv.Close()

	c := Client{Kind: "ollama", URL: srv.URL + "/api/embed", Model: "nomic-embed-text"}
	vecs, err := c.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 2 {
		t.Fatalf("vecs %+v", vecs)
	}
	if got["model"] != "nomic-embed-text" {
		t.Fatalf("model %v", got["model"])
	}
	in, _ := got["input"].([]any)
	if len(in) != 1 || in[0] != "hello" {
		t.Fatalf("input %v", got["input"])
	}
}

func TestOpenAIRequestShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{1, 2, 3}}},
		})
	}))
	defer srv.Close()
	c := Client{Kind: "openai", URL: srv.URL + "/v1/embeddings", Model: "nomic-embed-text"}
	vecs, err := c.Embed(context.Background(), []string{"hi"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("vecs %+v", vecs)
	}
}
