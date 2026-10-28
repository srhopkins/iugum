package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/contract"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// NativeHTTPProvider attaches only to an explicitly configured existing native listener.
// Closing a bridge releases its HTTP requests, never stops the native runtime.
type NativeHTTPProvider struct {
	BaseURL string
	Cwd     string
}

var nativeID = regexp.MustCompile(`^(main|[a-f0-9]{32})$`)

func (p NativeHTTPProvider) Name() string { return "Iugum" }
func (p NativeHTTPProvider) Capabilities() contract.AgentSessionCapabilities {
	return contract.AgentSessionCapabilities{Create: true, Resume: true, Interrupt: true, Approvals: true, Models: true}
}
func (p NativeHTTPProvider) Validate() error {
	u, err := url.Parse(p.BaseURL)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !filepath.IsAbs(p.Cwd) {
		return errors.New("native bridge requires loopback HTTP URL and absolute working directory")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("native URL must use a loopback IP")
	}
	return nil
}
func (p NativeHTTPProvider) call(ctx context.Context, method, path, id string, body, out any) error {
	if path != "/api/chat" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	if err := p.Validate(); err != nil {
		return err
	}
	var b io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		b = bytes.NewReader(data)
	}
	u, _ := url.Parse(p.BaseURL)
	u.Path = path
	if id != "" {
		u.RawQuery = url.Values{"conversation": []string{id}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// An explicit loopback IP bypasses environment proxies. Never follow redirects.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return errors.New("native request disconnected; inspect history before resending")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("native listener rejected request (HTTP %d)", response.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(out)
	}
	return nil
}
func (p NativeHTTPProvider) Catalog(ctx context.Context) ([]CatalogEntry, error) {
	var rows []struct{ ID, Title string }
	if err := p.call(ctx, "GET", "/api/conversations", "", nil, &rows); err != nil {
		return nil, err
	}
	out := []CatalogEntry{}
	for _, row := range rows {
		if nativeID.MatchString(row.ID) {
			out = append(out, entry(contract.AgentSessionRef{Provider: "Iugum", ID: row.ID, Cwd: p.Cwd, Title: row.Title}))
		}
	}
	return out, nil
}
func (p NativeHTTPProvider) Open(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (contract.AgentSession, error) {
	if ref.Provider != "Iugum" || ref.Cwd != p.Cwd || !nativeID.MatchString(ref.ID) {
		return nil, errors.New("native session identity mismatch")
	}
	rows, err := p.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ID == ref.ID {
			return p.attach(ctx, ref, o), nil
		}
	}
	return nil, errors.New("native conversation not found; no replacement created")
}
func (p NativeHTTPProvider) Create(ctx context.Context, q contract.AgentSessionCreate, o contract.AgentSessionObserver) (contract.AgentSessionRef, contract.AgentSession, error) {
	if q.Provider != "Iugum" || q.Cwd != p.Cwd || q.Model != "" {
		return contract.AgentSessionRef{}, nil, errors.New("native creation requires configured working directory and no global model change")
	}
	var result struct{ ID string }
	if err := p.call(ctx, "POST", "/api/conversations", "", map[string]any{}, &result); err != nil {
		return contract.AgentSessionRef{}, nil, err
	}
	if !nativeID.MatchString(result.ID) || result.ID == "main" {
		return contract.AgentSessionRef{}, nil, errors.New("native listener returned invalid new identity")
	}
	ref := contract.AgentSessionRef{Provider: "Iugum", ID: result.ID, Cwd: p.Cwd, Title: q.Title}
	return ref, p.attach(ctx, ref, o), nil
}
func (p NativeHTTPProvider) attach(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) *nativeHTTPSession {
	lifetime, cancel := context.WithCancel(ctx)
	s := &nativeHTTPSession{p: p, ref: ref, observer: o, ctx: lifetime, close: cancel}
	go s.approvals()
	return s
}

type nativeHTTPSession struct {
	p        NativeHTTPProvider
	ref      contract.AgentSessionRef
	observer contract.AgentSessionObserver
	ctx      context.Context
	close    context.CancelFunc
	mu       sync.Mutex
	turn     context.CancelFunc
}

func (s *nativeHTTPSession) History(ctx context.Context) ([]contract.AgentSessionMessage, error) {
	var out []contract.AgentSessionMessage
	err := s.p.call(ctx, "GET", "/api/messages", s.ref.ID, nil, &out)
	return out, err
}
func (s *nativeHTTPSession) Prompt(ctx context.Context, text string) error {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if s.turn != nil {
		s.mu.Unlock()
		cancel()
		return errors.New("native turn already running")
	}
	s.turn = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); s.turn = nil; s.mu.Unlock() }()
	s.observer.Event("state", "Native request running; progress is not streamed")
	return s.p.call(ctx, "POST", "/api/chat", s.ref.ID, map[string]string{"text": text}, nil)
}
func (s *nativeHTTPSession) Interrupt(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turn == nil {
		return errors.New("no active native request to interrupt")
	}
	s.turn()
	return nil
}
func (s *nativeHTTPSession) Close() error {
	s.close()
	s.mu.Lock()
	if s.turn != nil {
		s.turn()
	}
	s.mu.Unlock()
	return nil
}
func (s *nativeHTTPSession) Models(ctx context.Context) (contract.AgentSessionModels, error) {
	var result struct {
		Selected string                       `json:"selected"`
		Options  []contract.AgentSessionModel `json:"options"`
	}
	err := s.p.call(ctx, "GET", "/api/models", s.ref.ID, nil, &result)
	return contract.AgentSessionModels{Current: result.Selected, Options: result.Options, Reason: "Native profile selection affects the whole runtime. Use native model controls explicitly; this bridge never changes another conversation's model."}, err
}
func (s *nativeHTTPSession) approvals() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	seen := map[string]bool{}
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		var rows []struct {
			ID       string          `json:"id"`
			Digest   string          `json:"digest"`
			Status   string          `json:"status"`
			Title    string          `json:"title"`
			Action   string          `json:"action"`
			Payload  json.RawMessage `json:"payload"`
			Evidence json.RawMessage `json:"evidence"`
		}
		err := s.p.call(ctx, "GET", "/api/approvals", s.ref.ID, nil, &rows)
		cancel()
		if err != nil {
			continue
		}
		for _, row := range rows {
			key := row.ID + ":" + row.Digest
			if row.Status != "pending" || seen[key] {
				continue
			}
			seen[key] = true
			go func(id, digest string, params any) {
				b, _ := json.Marshal(params)
				result, err := s.observer.Request(s.ctx, contract.AgentSessionRequest{Method: "iugum/requestApproval", Params: b})
				if err != nil {
					return
				}
				var answer struct {
					Decision string `json:"decision"`
				}
				if json.Unmarshal(result, &answer) != nil {
					return
				}
				decision := "reject"
				if answer.Decision == "accept" {
					decision = "approve"
				} else if answer.Decision != "decline" && answer.Decision != "cancel" {
					return
				}
				ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
				defer cancel()
				if err := s.p.call(ctx, "POST", "/api/approvals/"+url.PathEscape(id), s.ref.ID, map[string]string{"decision": decision, "digest": digest}, nil); err != nil {
					s.observer.Event("error", "Native approval outcome uncertain; inspect original conversation")
				}
			}(row.ID, row.Digest, row)
		}
	}
}
