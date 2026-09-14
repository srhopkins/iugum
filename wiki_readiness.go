package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Do not advertise the proxy until its wiki can serve a page. API readiness
// alone is insufficient: the browser cannot recover from an initial 502 page.
func waitWikiReady(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := http.Client{Timeout: time.Second}
	timer := time.NewTicker(50 * time.Millisecond)
	defer timer.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wiki did not become ready: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
