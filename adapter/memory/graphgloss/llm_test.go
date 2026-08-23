package graphgloss

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractLLMDropsUnknownRel(t *testing.T) {
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
		if req["format"] == nil {
			t.Fatal("want json schema format")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{
				"content": `{"triples":[{"from":"steve","rel":"owns","to":"mi50"},{"from":"steve","rel":"invented","to":"mi50"}]}`,
			},
		})
	}))
	defer srv.Close()

	f := Default()
	c := LLMClient{Kind: "ollama", URL: srv.URL + "/api/chat", Model: "test-model"}
	got, err := f.ExtractLLM(context.Background(), c, "Steve owns the MI50.")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[0].From != "steve" || got[0].Rel != "owns" || got[0].To != "mi50" {
		t.Fatalf("edge %+v", got[0])
	}
}

func TestExtractLLMOpenAIPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{
					"content": `{"triples":[{"from":"tower","rel":"hosts","to":"mi50"}]}`,
				},
			}},
		})
	}))
	defer srv.Close()

	f := Default()
	c := LLMClient{Kind: "openai", URL: srv.URL + "/v1/chat/completions", Model: "test-model"}
	got, err := f.ExtractLLM(context.Background(), c, "The tower hosts the MI50.")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Rel != "hosts" || got[0].To != "mi50" {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractLLMMapsInflection(t *testing.T) {
	f := Default()
	raw := []byte(`{"triples":[{"from":"mi50","rel":"in","to":"tower"}]}`)
	got := f.filterLLMTriples(raw, "The MI50 is in the tower.")
	if len(got) != 1 || got[0].Rel != "located-in" || got[0].To != "tower" {
		t.Fatalf("got %+v", got)
	}
}
