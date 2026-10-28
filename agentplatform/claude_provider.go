package agentplatform

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/agentcontrol"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ClaudeProvider uses Go-managed Claude stream-json sessions. The caller supplies
// an account-isolated manager and explicit subscription identity/configuration.
type ClaudeProvider struct {
	Manager                          *agentcontrol.Manager
	Actor, Account, ConfigDir, Model string
}

func (ClaudeProvider) Name() string { return "Claude" }
func (ClaudeProvider) Capabilities() contract.AgentSessionCapabilities {
	return contract.AgentSessionCapabilities{Create: true, Models: true, Resume: true, Stream: true, Interrupt: true, Approvals: true}
}
func (p ClaudeProvider) Open(ctx context.Context, ref contract.AgentSessionRef, observer contract.AgentSessionObserver) (contract.AgentSession, error) {
	if err := validRef(ref); err != nil {
		return nil, err
	}
	if ref.Provider != "Claude" || p.Manager == nil || p.Actor == "" || p.Account == "" || !filepath.IsAbs(p.ConfigDir) {
		return nil, errors.New("Claude requires manager, actor, account and absolute config directory")
	}
	s := &claudeSession{provider: p, ref: ref, observer: observer}
	// Verify saved history before acquiring a writer. Missing sessions never become new conversations.
	history, err := s.savedHistory()
	if err != nil {
		return nil, err
	}
	s.history = history
	_, err = p.Manager.Start(ctx, agentcontrol.StartRequest{Actor: p.Actor, ID: ref.ID, ResumeSessionID: ref.ID, Project: ref.Cwd, Account: p.Account, ConfigDir: p.ConfigDir, Model: p.Model, StrictResume: true, Observer: s})
	if err != nil {
		return nil, err
	}
	return s, nil
}

type claudeSession struct {
	provider   ClaudeProvider
	ref        contract.AgentSessionRef
	observer   contract.AgentSessionObserver
	mu         sync.Mutex
	history    []contract.AgentSessionMessage
	completion chan error
	closed     bool
}

var claudePathEscape = regexp.MustCompile(`[^a-zA-Z0-9]`)

func (s *claudeSession) savedHistory() ([]contract.AgentSessionMessage, error) {
	path := filepath.Join(s.provider.ConfigDir, "projects", claudePathEscape.ReplaceAllString(s.ref.Cwd, "-"), s.ref.ID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result := []contract.AgentSessionMessage{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var row struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
			Message   struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}
		if row.SessionID != "" && row.SessionID != s.ref.ID || row.Cwd != "" && row.Cwd != s.ref.Cwd {
			return nil, errors.New("Claude history session identity mismatch")
		}
		if row.Type != "user" && row.Type != "assistant" {
			continue
		}
		text := claudeContent(row.Message.Content)
		if text != "" {
			result = append(result, contract.AgentSessionMessage{Role: row.Message.Role, Text: text})
		}
	}
	return result, scanner.Err()
}
func claudeContent(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct{ Type, Text string }
	_ = json.Unmarshal(raw, &blocks)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
func (s *claudeSession) Event(kind, text string) {
	if kind != "claude_frame" {
		return
	}
	var f struct {
		Type    string `json:"type"`
		Result  string `json:"result"`
		IsError bool   `json:"is_error"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		Event struct {
			Type  string                      `json:"type"`
			Delta struct{ Type, Text string } `json:"delta"`
		} `json:"event"`
	}
	if json.Unmarshal([]byte(text), &f) != nil {
		return
	}
	switch f.Type {
	case "stream_event":
		if f.Event.Delta.Type == "text_delta" {
			s.emit("text", f.Event.Delta.Text)
		} else {
			s.emit("progress", text)
		}
	case "assistant":
		value := claudeContent(f.Message.Content)
		s.mu.Lock()
		if value != "" {
			s.history = append(s.history, contract.AgentSessionMessage{Role: "assistant", Text: value})
		}
		s.mu.Unlock()
		s.emit("progress", text)
	case "tool_progress", "system":
		s.emit("progress", text)
	case "result":
		s.mu.Lock()
		ch := s.completion
		s.mu.Unlock()
		if ch != nil {
			var err error
			if f.IsError {
				err = errors.New(f.Result)
			}
			select {
			case ch <- err:
			default:
			}
		}
	}
}
func (s *claudeSession) emit(kind, text string) {
	if s.observer != nil {
		s.observer.Event(kind, text)
	}
}
func (s *claudeSession) Request(ctx context.Context, r contract.AgentSessionRequest) (json.RawMessage, error) {
	if s.observer == nil {
		return nil, errors.New("explicit client permission required")
	}
	return s.observer.Request(ctx, r)
}
func (s *claudeSession) History(ctx context.Context) ([]contract.AgentSessionMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contract.AgentSessionMessage{}, s.history...), nil
}
func (s *claudeSession) Prompt(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 200000 {
		return errors.New("invalid prompt")
	}
	s.mu.Lock()
	if s.closed || s.completion != nil {
		s.mu.Unlock()
		return errors.New("Claude session closed or busy")
	}
	ch := make(chan error, 1)
	s.completion = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.completion = nil; s.mu.Unlock() }()
	s.mu.Lock()
	s.history = append(s.history, contract.AgentSessionMessage{Role: "user", Text: text})
	s.mu.Unlock()
	if _, err := s.provider.Manager.Send(ctx, s.provider.Actor, s.ref.ID, text); err != nil {
		return err
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-ch:
			return err
		case <-ctx.Done():
			_ = s.Close()
			return errors.New("Claude turn outcome uncertain; inspect history before resending")
		case <-ticker.C:
			st, err := s.provider.Manager.Snapshot(ctx, s.provider.Actor, s.ref.ID)
			if err != nil {
				return err
			}
			if st.Status == "stopped" || st.Status == "failed" || st.Status == "disconnected" {
				return errors.New("Claude disconnected; outcome uncertain; inspect history before resending")
			}
		}
	}
}
func (s *claudeSession) Interrupt(ctx context.Context) error {
	return s.provider.Manager.Interrupt(ctx, s.provider.Actor, s.ref.ID)
}
func (s *claudeSession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.provider.Manager.Stop(context.Background(), s.provider.Actor, s.ref.ID)
}

var _ contract.AgentSessionProvider = ClaudeProvider{}

func (p ClaudeProvider) Create(ctx context.Context, q contract.AgentSessionCreate, o contract.AgentSessionObserver) (contract.AgentSessionRef, contract.AgentSession, error) {
	ref := contract.AgentSessionRef{Provider: "Claude", Cwd: q.Cwd, Title: q.Title}
	if q.Provider != "Claude" || p.Manager == nil || p.Actor == "" || p.Account == "" || !filepath.IsAbs(p.ConfigDir) {
		return ref, nil, errors.New("Claude requires explicit account configuration")
	}
	if q.Model != "" && q.Model != p.Model {
		return ref, nil, errors.New("Claude creation accepts only the configured default model")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return ref, nil, err
	}
	id[6] = (id[6] & 15) | 64
	id[8] = (id[8] & 63) | 128
	ref.ID = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	s := &claudeSession{provider: p, ref: ref, observer: o, history: []contract.AgentSessionMessage{}}
	_, err := p.Manager.Start(ctx, agentcontrol.StartRequest{Actor: p.Actor, ID: ref.ID, NewSessionID: ref.ID, Project: ref.Cwd, Account: p.Account, ConfigDir: p.ConfigDir, Model: p.Model, Observer: s})
	if err != nil {
		return ref, nil, err
	}
	return ref, s, nil
}
func (s *claudeSession) Models(context.Context) (contract.AgentSessionModels, error) {
	out := contract.AgentSessionModels{Options: []contract.AgentSessionModel{}, Current: s.provider.Model, Reason: "Uses configured default model; dynamic model discovery and switching are unsupported."}
	if s.provider.Model != "" {
		out.Options = append(out.Options, contract.AgentSessionModel{ID: s.provider.Model, Name: s.provider.Model})
	}
	return out, nil
}
func (s *claudeSession) SelectModel(context.Context, string) error {
	return errors.New("Claude dynamic model switching unsupported")
}
