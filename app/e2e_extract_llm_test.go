package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestE2EExtractLLMGlossaryGate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatal(err)
		}
		opts, _ := req["options"].(map[string]any)
		if opts["temperature"] != float64(0) {
			t.Fatalf("temperature %v", opts["temperature"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{
				"content": `{"triples":[{"from":"steve","rel":"owns","to":"mi50"},{"from":"steve","rel":"invented","to":"mi50"}]}`,
			},
		})
	}))
	defer srv.Close()

	cfg := e2eConfig(t)
	cfg.Graph.Extractor = "llm"
	cfg.Graph.LLM.Kind = "ollama"
	cfg.Graph.LLM.URL = srv.URL + "/api/chat"
	cfg.Graph.LLM.Model = "test-model"
	cfg.Graph.Glossary = filepath.Join("..", "glossaries", "memory-graph.yaml")

	a := newE2EApp(t, cfg)
	ctx := context.Background()

	edges, err := a.Ingest(ctx, "lab", "Steve owns the MI50.")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges %+v", edges)
	}
	if edges[0].Rel != "owns" || edges[0].To != "mi50" {
		t.Fatalf("edge %+v", edges[0])
	}

	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range walk {
		if h.Rel == "owns" && h.To == "mi50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("walk %+v", walk)
	}
}
