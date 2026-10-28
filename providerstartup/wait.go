package providerstartup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// WaitReady supplies a boot dependency for clients that load AuthFile once.
// It never starts services or creates/changes a credential.
func WaitReady(ctx context.Context, s Settings) error {
	return waitReadyWith(ctx, s, "http://127.0.0.1:3853", "http://127.0.0.1:4097")
}
func waitReadyWith(ctx context.Context, s Settings, host, backend string) error {
	if e := validate(s); e != nil {
		return e
	}
	if s.Mode == "existing" {
		backend = s.ExistingURL
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	for {
		token, e := privateRead(s.TokenFile)
		if e == nil {
			req, _ := http.NewRequestWithContext(ctx, "GET", host+"/api/agents", nil)
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
			res, requestErr := client.Do(req)
			if requestErr == nil {
				io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
				res.Body.Close()
				if res.StatusCode == 200 {
					pw, readErr := privateRead(s.PasswordFile)
					if readErr == nil && verifyExisting(ctx, backend, pw) == nil {
						return nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("provider startup not ready; native launcher kept stopped to avoid stale editor authentication")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
