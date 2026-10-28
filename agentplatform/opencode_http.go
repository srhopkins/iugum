package agentplatform

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/srhopkins/iugum/contract"
)

// OpenCodeHTTPProvider connects to an already running, authenticated loopback
// OpenCode server. It never starts a process or substitutes a conversation.
// Wire references: docs.opencode.ai/docs/server and the generated v2 SDK.
type OpenCodeHTTPProvider struct {
	BaseURL, Username, Password string
	Client                      *http.Client
}

func (OpenCodeHTTPProvider) Name() string { return "OpenCode" }
func (OpenCodeHTTPProvider) Capabilities() contract.AgentSessionCapabilities {
	return contract.AgentSessionCapabilities{Create: true, Models: true, SelectModel: true, Resume: true, Stream: true, Interrupt: true, Approvals: true}
}

type openCodeHTTPSession struct {
	model            string
	stopping         bool
	stopAcknowledged bool
	stopIdle         bool
	p                OpenCodeHTTPProvider
	client           *http.Client
	ref              contract.AgentSessionRef
	observer         contract.AgentSessionObserver
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	done             chan error
	user             string
	assistants       map[string]bool
	closed           bool
	turnCtx          context.Context
	turnCancel       context.CancelFunc
	requests         map[string]bool
}

func (p OpenCodeHTTPProvider) Open(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (contract.AgentSession, error) {
	if err := validRef(ref); err != nil {
		return nil, err
	}
	if ref.Provider != "OpenCode" {
		return nil, errors.New("provider mismatch")
	}
	s, err := p.connection(ctx, ref, o)
	if err != nil {
		return nil, err
	}
	cancel := s.cancel
	life := s.ctx
	var info struct{ ID, Directory string }
	if err = s.call(ctx, "GET", "/session/"+ref.ID, nil, &info); err != nil {
		cancel()
		return nil, err
	}
	if info.ID != ref.ID || info.Directory != ref.Cwd {
		cancel()
		return nil, errors.New("OpenCode returned a different session or directory")
	}
	res, err := s.do(life, "GET", "/event", nil)
	if err != nil {
		cancel()
		return nil, err
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		res.Body.Close()
		cancel()
		return nil, errors.New("OpenCode event endpoint is not SSE")
	}
	go s.stream(res.Body)
	return s, nil
}
func (s *openCodeHTTPSession) do(ctx context.Context, method, path string, b any) (*http.Response, error) {
	var body io.Reader
	if b != nil {
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	u, _ := url.Parse(strings.TrimRight(s.p.BaseURL, "/") + path)
	q := u.Query()
	q.Set("directory", s.ref.Cwd)
	u.RawQuery = q.Encode()
	r, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, errors.New("invalid OpenCode request")
	}
	r.Header.Set("Content-Type", "application/json")
	if s.p.Password != "" {
		name := s.p.Username
		if name == "" {
			name = "opencode"
		}
		r.SetBasicAuth(name, s.p.Password)
	}
	res, err := s.client.Do(r)
	if err != nil {
		return nil, errors.New("OpenCode connection failed; delivery may be uncertain")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return nil, fmt.Errorf("OpenCode HTTP status %d", res.StatusCode)
	}
	return res, nil
}
func (s *openCodeHTTPSession) call(ctx context.Context, m, p string, b, out any) error {
	r, e := s.do(ctx, m, p, b)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if out != nil {
		return json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(out)
	}
	return nil
}
func (s *openCodeHTTPSession) History(ctx context.Context) ([]contract.AgentSessionMessage, error) {
	var rows []struct {
		Info  struct{ Role string }
		Parts []struct{ Type, Text string }
	}
	if err := s.call(ctx, "GET", "/session/"+s.ref.ID+"/message", nil, &rows); err != nil {
		return nil, err
	}
	out := []contract.AgentSessionMessage{}
	for _, r := range rows {
		if r.Info.Role != "user" && r.Info.Role != "assistant" {
			continue
		}
		var text []string
		for _, p := range r.Parts {
			if p.Type == "text" {
				text = append(text, p.Text)
			}
		}
		out = append(out, contract.AgentSessionMessage{Role: r.Info.Role, Text: strings.Join(text, "\n")})
	}
	return out, nil
}
func (s *openCodeHTTPSession) Prompt(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 200000 {
		return errors.New("invalid prompt")
	}
	var id [16]byte
	if _, e := rand.Read(id[:]); e != nil {
		return e
	}
	s.mu.Lock()
	if s.closed || s.done != nil {
		s.mu.Unlock()
		return errors.New("OpenCode session closed or busy")
	}
	s.stopping = false
	s.stopAcknowledged = false
	s.stopIdle = false
	s.turnCtx, s.turnCancel = context.WithCancel(s.ctx)
	s.requests = map[string]bool{}
	s.done = make(chan error, 1)
	done := s.done
	s.user = "msg_" + hex.EncodeToString(id[:])
	s.assistants = map[string]bool{}
	user := s.user
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.turnCancel(); s.done = nil; s.user = ""; s.mu.Unlock() }()
	params := map[string]any{"messageID": user, "parts": []any{map[string]string{"type": "text", "text": text}}}
	s.mu.Lock()
	model := s.model
	s.mu.Unlock()
	if model != "" {
		ids := strings.SplitN(model, "/", 2)
		params["model"] = map[string]string{"providerID": ids[0], "modelID": ids[1]}
	}
	if err := s.call(ctx, "POST", "/session/"+s.ref.ID+"/prompt_async", params, nil); err != nil {
		return err
	}
	select {
	case e := <-done:
		return e
	case <-ctx.Done():
		s.Close()
		return fmt.Errorf("OpenCode turn outcome uncertain: %w", ctx.Err())
	case <-s.ctx.Done():
		return errors.New("OpenCode disconnected; turn outcome uncertain; inspect history before resending")
	}
}
func (s *openCodeHTTPSession) Interrupt(ctx context.Context) error {
	s.mu.Lock()
	done := s.done
	if done == nil {
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	s.mu.Unlock()
	err := s.call(ctx, "POST", "/session/"+s.ref.ID+"/abort", map[string]any{}, nil)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != done {
		return err
	}
	if err != nil {
		s.stopping = false
		return err
	}
	s.stopAcknowledged = true
	if s.stopIdle {
		s.finish(nil)
	}
	return nil
}

func (s *openCodeHTTPSession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	return nil
}
func (s *openCodeHTTPSession) finish(e error) {
	if s.turnCancel != nil {
		s.turnCancel()
	}
	if s.done != nil {
		select {
		case s.done <- e:
		default:
		}
	}
}
func (s *openCodeHTTPSession) stream(body io.ReadCloser) {
	defer body.Close()
	defer s.Close()
	scan := bufio.NewScanner(body)
	scan.Buffer(make([]byte, 4096), 2<<20)
	var data []string
	for scan.Scan() {
		line := scan.Text()
		if line == "" {
			if len(data) > 0 {
				s.handle([]byte(strings.Join(data, "\n")))
				data = nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
}
func (s *openCodeHTTPSession) handle(raw []byte) {
	var e struct {
		Type       string
		Properties json.RawMessage
	}
	if json.Unmarshal(raw, &e) != nil {
		return
	}
	var p struct {
		SessionID, ID, MessageID, Field, Delta string
		Info                                   struct{ ID, SessionID, Role, ParentID string }
		Part                                   struct {
			SessionID, MessageID, Type string
			State                      struct{ Status string }
		}
		Status struct{ Type string }
	}
	if json.Unmarshal(e.Properties, &p) != nil {
		return
	}
	sid := p.SessionID
	if sid == "" {
		sid = p.Info.SessionID
	}
	if sid == "" {
		sid = p.Part.SessionID
	}
	if sid != s.ref.ID {
		return
	}
	s.mu.Lock()
	active := s.done != nil
	switch e.Type {
	case "message.updated":
		if active && p.Info.Role == "assistant" && p.Info.ParentID == s.user {
			s.assistants[p.Info.ID] = true
		}
	case "message.part.delta":
		if active && s.assistants[p.MessageID] && p.Field == "text" && s.observer != nil {
			s.mu.Unlock()
			s.observer.Event("text", p.Delta)
			return
		}
	case "message.part.updated":
		if active && s.assistants[p.Part.MessageID] && p.Part.Type == "tool" && s.observer != nil {
			s.mu.Unlock()
			s.observer.Event("tool", p.Part.State.Status)
			return
		}
	case "session.idle", "session.status":
		if active && (e.Type == "session.idle" || p.Status.Type == "idle") {
			if s.stopping {
				s.stopIdle = true
			}
			if len(s.assistants) > 0 || s.stopping && s.stopAcknowledged {
				s.finish(nil)
			}
		}
	case "session.error":
		if active {
			s.finish(errors.New("OpenCode session failed; inspect history before resending"))
		}
	case "permission.asked", "question.asked":
		if active && s.observer != nil && p.ID != "" && !strings.ContainsAny(p.ID, "/\\?#") {
			if !s.requests[p.ID] {
				s.requests[p.ID] = true
				go s.reply(s.turnCtx, e.Type, p.ID, e.Properties)
			}
		}
	}
	s.mu.Unlock()
}
func (s *openCodeHTTPSession) reply(lifetime context.Context, kind, id string, params json.RawMessage) {
	ctx, cancel := context.WithCancel(lifetime)
	defer cancel()
	response, err := s.observer.Request(ctx, contract.AgentSessionRequest{ID: id, Method: kind, Params: params})
	if err != nil {
		return
	}
	if ctx.Err() != nil || validateNativeAnswer(contract.AgentSessionRequest{Method: kind, Params: params}, response) != nil {
		return
	}
	var b map[string]json.RawMessage
	if json.Unmarshal(response, &b) != nil {
		return
	}
	path := "/permission/" + id + "/reply"
	if kind == "permission.asked" {
		var v string
		if len(b) != 1 || json.Unmarshal(b["reply"], &v) != nil || (v != "once" && v != "reject") {
			return
		}
	} else {
		path = "/question/" + id + "/reply"
		if string(b["reject"]) == "true" {
			_ = s.call(ctx, "POST", "/question/"+id+"/reject", map[string]any{}, nil)
			return
		}
		var answers [][]string
		if len(b) != 1 || json.Unmarshal(b["answers"], &answers) != nil || answers == nil {
			return
		}
	}
	if err = s.call(ctx, "POST", path, b, nil); err != nil && s.observer != nil {
		s.observer.Event("error", "OpenCode decision delivery failed; inspect pending requests")
	}
}

var _ contract.AgentSessionProvider = OpenCodeHTTPProvider{}

func (p OpenCodeHTTPProvider) connection(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (*openCodeHTTPSession, error) {
	u, err := url.Parse(p.BaseURL)
	if err != nil || u == nil || u.Scheme != "http" && u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid OpenCode server URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("OpenCode server must use loopback")
	}
	c := http.Client{}
	if p.Client != nil {
		c = *p.Client
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	life, cancel := context.WithCancel(ctx)
	s := &openCodeHTTPSession{p: p, client: &c, ref: ref, observer: o, ctx: life, cancel: cancel}
	return s, nil
}
