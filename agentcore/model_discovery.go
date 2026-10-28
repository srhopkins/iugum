package agentcore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DiscoverProfiles expands explicitly opted-in OpenAI-compatible pools before
// runtime construction. Explicit profiles keep their prices, limits and policies.
// Discovery never claims that a model is free or supports images.
func DiscoverProfiles(ctx context.Context, c *Config) error {
	for name, pool := range c.Pools {
		if !pool.DiscoverModels {
			continue
		}
		models, err := DiscoverModels(ctx, pool)
		if err != nil {
			return fmt.Errorf("model catalogue for pool %q: %w", name, err)
		}
		if c.Profiles == nil {
			c.Profiles = map[string]Profile{}
		}
		for _, model := range models {
			exists := false
			for _, p := range c.Profiles {
				if p.Pool == name && p.Model == model {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			hash := sha256.Sum256([]byte(name + "\x00" + model))
			id := "discovered-" + hex.EncodeToString(hash[:16])
			if _, exists := c.Profiles[id]; exists {
				return fmt.Errorf("discovered profile conflicts with configured profile")
			}
			c.Profiles[id] = Profile{Pool: name, Model: model}
		}
	}
	return nil
}

// DiscoverModels uses the same backend credential references as inference.
// Redirects are rejected so credentials cannot move to another endpoint.
func DiscoverModels(ctx context.Context, pool Pool) ([]string, error) {
	u, err := url.Parse(pool.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("a plain HTTP(S) base URL is required")
	}
	key := ""
	if pool.APIKeyFile != "" {
		raw, err := os.ReadFile(pool.APIKeyFile)
		if err != nil || strings.TrimSpace(string(raw)) == "" {
			return nil, fmt.Errorf("configured credential file is unavailable or empty")
		}
		key = strings.TrimSpace(string(raw))
	}
	if pool.APIKeyEnv != "" {
		key = os.Getenv(pool.APIKeyEnv)
		if key == "" {
			return nil, fmt.Errorf("configured credential environment variable is unset")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create catalogue request")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach model catalogue")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model catalogue returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil || len(raw) > 2*1024*1024 {
		return nil, fmt.Errorf("model catalogue exceeds response limit")
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Data == nil || len(result.Data) > 4096 {
		return nil, fmt.Errorf("invalid model catalogue")
	}
	models := []string{}
	seen := map[string]bool{}
	for _, item := range result.Data {
		if strings.TrimSpace(item.ID) == "" || len(item.ID) > 256 || strings.ContainsAny(item.ID, "\r\n\x00") {
			return nil, fmt.Errorf("invalid model identifier")
		}
		if !seen[item.ID] {
			models = append(models, item.ID)
			seen[item.ID] = true
		}
	}
	return models, nil
}
