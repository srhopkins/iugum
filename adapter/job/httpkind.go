package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/srhopkins/iugum/contract"
)

type httpEventBody struct {
	Event  string            `json:"event"`
	Source string            `json:"source"`
	Path   string            `json:"path"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

// HTTPPost returns a JobFunc that POSTs the event as JSON to url.
// The request honors ctx cancellation and deadlines.
func HTTPPost(url string) contract.JobFunc {
	u := url
	return func(ctx context.Context, ev contract.Event) error {
		if u == "" {
			return fmt.Errorf("iugum: http job: empty url")
		}
		body, err := json.Marshal(httpEventBody{
			Event:  ev.Name,
			Source: ev.Source,
			Path:   ev.Path,
			Attrs:  ev.Attrs,
		})
		if err != nil {
			return fmt.Errorf("iugum: http job: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("iugum: http job: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("iugum: http job: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return fmt.Errorf("iugum: http job: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
		}
		return nil
	}
}
