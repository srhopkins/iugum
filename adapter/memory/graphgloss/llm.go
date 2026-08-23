package graphgloss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/srhopkins/iugum/contract"
)

// LLMClient calls Ollama or OpenAI-compat chat for structured triple extract.
type LLMClient struct {
	Kind    string // ollama | openai
	URL     string
	Model   string
	Timeout time.Duration
	Do      func(*http.Request) (*http.Response, error)
}

func (c LLMClient) client() func(*http.Request) (*http.Response, error) {
	if c.Do != nil {
		return c.Do
	}
	t := c.Timeout
	if t == 0 {
		t = 60 * time.Second
	}
	h := &http.Client{Timeout: t}
	return h.Do
}

// ExtractLLM asks a chat model for triples, then drops any rel not in the glossary.
func (f File) ExtractLLM(ctx context.Context, c LLMClient, text string) ([]contract.MemoryEdge, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	raw, err := c.chat(ctx, f, text)
	if err != nil {
		return nil, err
	}
	return f.filterLLMTriples(raw, text), nil
}

func (f File) filterLLMTriples(raw []byte, source string) []contract.MemoryEdge {
	var parsed struct {
		Triples []struct {
			From string `json:"from"`
			Rel  string `json:"rel"`
			To   string `json:"to"`
		} `json:"triples"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	var out []contract.MemoryEdge
	for _, t := range parsed.Triples {
		canon, ok := f.CanonRel(t.Rel)
		if !ok {
			continue
		}
		from := slug(t.From)
		to := slug(t.To)
		if from == "" || to == "" || from == to {
			continue
		}
		out = append(out, contract.MemoryEdge{From: from, Rel: canon, To: to, Value: source})
	}
	return out
}

func (c LLMClient) chat(ctx context.Context, gloss File, text string) ([]byte, error) {
	if c.Kind == "openai" {
		return c.chatOpenAI(ctx, gloss, text)
	}
	return c.chatOllama(ctx, gloss, text)
}

func (c LLMClient) chatOllama(ctx context.Context, gloss File, text string) ([]byte, error) {
	url := c.URL
	if url == "" {
		url = "http://127.0.0.1:11434/api/chat"
	}
	model := c.Model
	if model == "" {
		model = "qwen3.5:4b"
	}
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": llmPrompt(gloss, text)},
		},
		"stream": false,
		"format": tripleSchema(gloss),
		"options": map[string]any{
			"temperature": 0,
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client()(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("extract: ollama %s: %s", res.Status, truncateLLM(raw))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return []byte(out.Message.Content), nil
}

func (c LLMClient) chatOpenAI(ctx context.Context, gloss File, text string) ([]byte, error) {
	url := c.URL
	if url == "" {
		url = "http://127.0.0.1:11434/v1/chat/completions"
	}
	model := c.Model
	if model == "" {
		model = "qwen3.5:4b"
	}
	schema := tripleSchema(gloss)
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": llmPrompt(gloss, text)},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "triples",
				"strict": true,
				"schema": schema,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client()(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("extract: openai %s: %s", res.Status, truncateLLM(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("extract: openai: empty choices")
	}
	return []byte(out.Choices[0].Message.Content), nil
}

func llmPrompt(gloss File, text string) string {
	var rels []string
	for _, t := range gloss.Rels {
		rels = append(rels, t.Word)
	}
	return "Extract knowledge-graph triples from the text. Use only these rel values: " +
		strings.Join(rels, ", ") + ".\n\nText:\n" + text
}

func tripleSchema(gloss File) map[string]any {
	enum := make([]string, 0, len(gloss.Rels))
	for _, t := range gloss.Rels {
		enum = append(enum, t.Word)
		for _, inf := range t.Inflections {
			enum = append(enum, inf)
		}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"triples": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"from": map[string]any{"type": "string"},
						"rel":  map[string]any{"type": "string", "enum": enum},
						"to":   map[string]any{"type": "string"},
					},
					"required": []string{"from", "rel", "to"},
				},
			},
		},
		"required": []string{"triples"},
	}
}

func truncateLLM(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
