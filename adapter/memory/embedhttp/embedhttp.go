package embedhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls an HTTP embedding server. Kind is ollama or openai.
type Client struct {
	Kind    string // ollama | openai
	URL     string
	Model   string
	Timeout time.Duration
	Do      func(*http.Request) (*http.Response, error)
}

func (c Client) client() func(*http.Request) (*http.Response, error) {
	if c.Do != nil {
		return c.Do
	}
	t := c.Timeout
	if t == 0 {
		t = 30 * time.Second
	}
	h := &http.Client{Timeout: t}
	return h.Do
}

func (c Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if c.Kind == "openai" {
		return c.embedOpenAI(ctx, texts)
	}
	return c.embedOllama(ctx, texts)
}

func (c Client) embedOllama(ctx context.Context, texts []string) ([][]float32, error) {
	url := c.URL
	if url == "" {
		url = "http://127.0.0.1:11434/api/embed"
	}
	model := c.Model
	if model == "" {
		model = "nomic-embed-text"
	}
	body, err := json.Marshal(map[string]any{"model": model, "input": texts})
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
		return nil, fmt.Errorf("embed: ollama %s: %s", res.Status, truncate(raw))
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Embeddings, nil
}

func (c Client) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	url := c.URL
	if url == "" {
		url = "http://127.0.0.1:11434/v1/embeddings"
	}
	model := c.Model
	if model == "" {
		model = "nomic-embed-text"
	}
	body, err := json.Marshal(map[string]any{"model": model, "input": texts})
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
		return nil, fmt.Errorf("embed: openai %s: %s", res.Status, truncate(raw))
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

func truncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
