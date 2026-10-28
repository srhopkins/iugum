package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
)

var uuid = regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var openCodeID = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

type Provider struct {
	Provider string
	Binary   string
}

func (p Provider) Name() string { return p.Provider }
func (p Provider) Capabilities() contract.AgentSessionCapabilities {
	return contract.AgentSessionCapabilities{Create: p.Provider == "Codex" || p.Provider == "Cursor", Models: true, SelectModel: true, Resume: true, Stream: true, Interrupt: true, Approvals: true}
}
func validRef(ref contract.AgentSessionRef) error {
	if ref.Provider != "Codex" && ref.Provider != "Claude" && ref.Provider != "OpenCode" && ref.Provider != "Cursor" && ref.Provider != "Iugum" {
		return errors.New("unknown agent provider")
	}
	if !(uuid.MatchString(ref.ID) || ref.Provider == "OpenCode" && openCodeID.MatchString(ref.ID) || ref.Provider == "Iugum" && nativeID.MatchString(ref.ID)) {
		return errors.New("invalid provider session ID")
	}
	if !filepath.IsAbs(ref.Cwd) {
		return errors.New("session requires its original absolute working directory")
	}
	return nil
}
func (p Provider) Open(ctx context.Context, ref contract.AgentSessionRef, observer contract.AgentSessionObserver) (contract.AgentSession, error) {
	return p.connect(ctx, ref, observer, false, "")
}
func (p Provider) connect(ctx context.Context, ref contract.AgentSessionRef, observer contract.AgentSessionObserver, create bool, model string) (contract.AgentSession, error) {
	if err := validRef(ref); err != nil && !create {
		return nil, err
	}
	if ref.Provider != p.Provider {
		return nil, errors.New("provider mismatch")
	}
	s := &session{ref: ref, observer: observer, history: []contract.AgentSessionMessage{}, lifetime: ctx}
	args := []string{}
	switch p.Provider {
	case "Codex":
		args = []string{"app-server", "--listen", "stdio://"}
	case "OpenCode":
		args = []string{"acp", "--cwd", ref.Cwd}
	case "Cursor":
		args = []string{"acp"}
	}
	r, err := startRPC(ctx, p.Binary, args, ref.Cwd, s.notify, s.request)
	if err != nil {
		return nil, err
	}
	s.rpc = r
	fail := func(err error) (contract.AgentSession, error) { r.close(); return nil, err }
	if p.Provider == "Codex" {
		if err = r.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "iugum_sessions", "version": "0.1.0"}}, nil); err != nil {
			return fail(err)
		}
		if err = r.notification("initialized", map[string]any{}); err != nil {
			return fail(err)
		}
		if create {
			var result struct {
				Thread struct {
					ID  string `json:"id"`
					Cwd string `json:"cwd"`
				} `json:"thread"`
				Model string `json:"model"`
			}
			params := map[string]any{"cwd": ref.Cwd}
			if model != "" {
				if err = s.SelectModel(ctx, model); err != nil {
					return fail(err)
				}
				params["model"] = model
			}
			if err = r.call(ctx, "thread/start", params, &result); err != nil {
				return fail(err)
			}
			if result.Thread.Cwd != ref.Cwd {
				return fail(errors.New("Codex created session directory mismatch"))
			}
			s.ref.ID = result.Thread.ID
			s.model = result.Model
			if s.model == "" {
				s.model = model
			}
			s.resumed = true
			if err = validRef(s.ref); err != nil {
				return fail(err)
			}
			return s, nil
		}
		// Read history first; acquire the provider writer only when a prompt is sent.
		if _, err = s.History(ctx); err != nil {
			return fail(err)
		}
	} else {
		var init struct {
			ProtocolVersion   int `json:"protocolVersion"`
			AgentCapabilities struct {
				LoadSession bool `json:"loadSession"`
			} `json:"agentCapabilities"`
		}
		if err = r.call(ctx, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}, "clientInfo": map[string]string{"name": "iugum_sessions", "version": "0.1.0"}}, &init); err != nil {
			return fail(err)
		}
		if init.ProtocolVersion != 1 || !create && !init.AgentCapabilities.LoadSession {
			return fail(errors.New("agent does not support loading existing ACP sessions"))
		}
		var loaded struct {
			SessionID     string            `json:"sessionId"`
			ConfigOptions []acpConfigOption `json:"configOptions"`
		}
		method := "session/load"
		params := map[string]any{"sessionId": ref.ID, "cwd": ref.Cwd, "mcpServers": []any{}}
		if create {
			method = "session/new"
			delete(params, "sessionId")
		}
		if err = r.call(ctx, method, params, &loaded); err != nil {
			return fail(err)
		}
		s.configOptions = loaded.ConfigOptions
		if create {
			s.ref.ID = loaded.SessionID
			if err = validRef(s.ref); err != nil {
				return fail(err)
			}
			if model != "" {
				if err = s.SelectModel(ctx, model); err != nil {
					return fail(err)
				}
			}
			return s, nil
		}
		if loaded.SessionID != "" && loaded.SessionID != ref.ID {
			return fail(errors.New("agent returned a different session; refusing replacement"))
		}
	}
	return s, nil
}

type session struct {
	model         string
	configOptions []acpConfigOption
	rpc           *rpc
	ref           contract.AgentSessionRef
	observer      contract.AgentSessionObserver
	lifetime      context.Context
	mu            sync.Mutex
	history       []contract.AgentSessionMessage
	busy          bool
	resumed       bool
	turnID        string
	completion    chan turnCompletion
	boundary      bool
}

type turnCompletion struct {
	id  string
	err error
}

func (s *session) event(kind, text string) {
	if s.observer != nil {
		s.observer.Event(kind, text)
	}
}
func (s *session) notify(f frame) {
	var p struct {
		SessionID string `json:"sessionId"`
		ThreadID  string `json:"threadId"`
		Delta     string `json:"delta"`
		Item      struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"item"`
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
		Update struct {
			Kind    string `json:"sessionUpdate"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ToolCallID    string            `json:"toolCallId"`
			Title         string            `json:"title"`
			Status        string            `json:"status"`
			ConfigOptions []acpConfigOption `json:"configOptions"`
		} `json:"update"`
	}
	if json.Unmarshal(f.Params, &p) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ref.Provider == "Codex" {
		if p.ThreadID != s.ref.ID {
			return
		}
		switch f.Method {
		case "item/started", "item/completed":
			if p.Item.Type == "commandExecution" || p.Item.Type == "fileChange" || p.Item.Type == "mcpToolCall" {
				s.event("tool", strings.TrimSpace(p.Item.Type+" "+p.Item.Command))
			}
		case "item/agentMessage/delta":
			s.event("text", p.Delta)
		case "turn/completed":
			// Match completion against the turn/start response in Prompt.
			var err error
			if p.Turn.Status == "failed" {
				err = errors.New("Codex turn failed")
				if p.Turn.Error != nil {
					err = errors.New(p.Turn.Error.Message)
				}
			}
			if s.completion != nil {
				select {
				case s.completion <- turnCompletion{id: p.Turn.ID, err: err}:
				default:
				}
			}
		}
		return
	}
	if f.Method != "session/update" || p.SessionID != s.ref.ID {
		return
	}
	u := p.Update
	if u.Kind == "config_option_update" {
		s.configOptions = u.ConfigOptions
		return
	}
	if u.Kind == "tool_call" || u.Kind == "tool_call_update" {
		s.boundary = true
		s.event("tool", strings.TrimSpace(u.Title+" "+u.Status))
		return
	}
	if (u.Kind != "agent_message_chunk" && u.Kind != "user_message_chunk") || u.Content.Type != "text" {
		return
	}
	role := "assistant"
	if u.Kind == "user_message_chunk" {
		role = "user"
	}
	if n := len(s.history); n > 0 && !s.boundary && s.history[n-1].Role == role {
		s.history[n-1].Text += u.Content.Text
	} else {
		s.history = append(s.history, contract.AgentSessionMessage{Role: role, Text: u.Content.Text})
	}
	s.boundary = false
	if s.busy && role == "assistant" {
		s.event("text", u.Content.Text)
	}
}
func (s *session) request(f frame) (json.RawMessage, error) {
	var identity struct {
		SessionID string `json:"sessionId"`
		ThreadID  string `json:"threadId"`
	}
	if err := json.Unmarshal(f.Params, &identity); err != nil {
		return nil, err
	}
	if s.ref.Provider == "Codex" {
		if identity.ThreadID != s.ref.ID {
			return nil, errors.New("foreign thread request")
		}
	} else if identity.SessionID != s.ref.ID && (identity.SessionID != "" || f.Method == "session/request_permission") {
		return nil, errors.New("foreign session request")
	}
	allowed := f.Method == "session/request_permission" || s.ref.Provider == "Codex" && (f.Method == "item/commandExecution/requestApproval" || f.Method == "item/fileChange/requestApproval" || f.Method == "item/tool/requestUserInput") || s.ref.Provider == "Cursor" && (f.Method == "cursor/ask_question" || f.Method == "cursor/create_plan")
	if !allowed || s.observer == nil {
		return nil, errors.New("unsupported client capability; no approval granted")
	}
	ctx, cancel := context.WithTimeout(s.lifetime, 10*time.Minute)
	defer cancel()
	return s.observer.Request(ctx, contract.AgentSessionRequest{ID: string(f.ID), Method: f.Method, Params: f.Params})
}
func (s *session) History(ctx context.Context) ([]contract.AgentSessionMessage, error) {
	if s.ref.Provider == "Codex" {
		var result struct {
			Thread struct {
				ID    string `json:"id"`
				Cwd   string `json:"cwd"`
				Turns []struct {
					Items []struct {
						Type    string `json:"type"`
						Text    string `json:"text"`
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					} `json:"items"`
				} `json:"turns"`
			} `json:"thread"`
		}
		if err := s.rpc.call(ctx, "thread/read", map[string]any{"threadId": s.ref.ID, "includeTurns": true}, &result); err != nil {
			return nil, err
		}
		if result.Thread.Cwd != s.ref.Cwd {
			return nil, errors.New("Codex original session directory mismatch")
		}
		if result.Thread.ID != s.ref.ID {
			return nil, errors.New("Codex returned a different thread")
		}
		messages := []contract.AgentSessionMessage{}
		for _, turn := range result.Thread.Turns {
			for _, item := range turn.Items {
				switch item.Type {
				case "userMessage":
					parts := []string{}
					for _, c := range item.Content {
						parts = append(parts, c.Text)
					}
					messages = append(messages, contract.AgentSessionMessage{Role: "user", Text: strings.Join(parts, "\n")})
				case "agentMessage":
					messages = append(messages, contract.AgentSessionMessage{Role: "assistant", Text: item.Text})
				}
			}
		}
		return messages, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contract.AgentSessionMessage{}, s.history...), nil
}
func (s *session) Prompt(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 200000 {
		return errors.New("invalid prompt")
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errors.New("session is already working")
	}
	s.busy = true
	s.turnID = ""
	s.boundary = true
	s.completion = make(chan turnCompletion, 64)
	done := s.completion
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.completion = nil; s.turnID = ""; s.mu.Unlock() }()
	if s.ref.Provider != "Codex" {
		return s.rpc.call(ctx, "session/prompt", map[string]any{"sessionId": s.ref.ID, "prompt": []any{map[string]string{"type": "text", "text": text}}}, nil)
	}
	if !s.resumed {
		var result struct {
			Thread struct {
				ID  string `json:"id"`
				Cwd string `json:"cwd"`
			} `json:"thread"`
		}
		if err := s.rpc.call(ctx, "thread/resume", map[string]any{"threadId": s.ref.ID}, &result); err != nil {
			return err
		}
		if result.Thread.Cwd != s.ref.Cwd {
			return errors.New("Codex resumed session directory mismatch")
		}
		if result.Thread.ID != s.ref.ID {
			return errors.New("Codex resumed a different thread")
		}
		s.resumed = true
	}
	var result struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := s.rpc.call(ctx, "turn/start", map[string]any{"threadId": s.ref.ID, "model": s.modelOrNil(), "input": []any{map[string]string{"type": "text", "text": text}}}, &result); err != nil {
		return err
	}
	s.mu.Lock()
	if result.Turn.ID == "" {
		s.mu.Unlock()
		s.rpc.close()
		return errors.New("Codex omitted turn identity; outcome uncertain")
	}
	s.turnID = result.Turn.ID
	s.mu.Unlock()
	for {
		select {
		case completed := <-done:
			if completed.id != result.Turn.ID {
				continue
			}
			return completed.err
		case <-s.rpc.done:
			return errors.New("connection lost; turn outcome uncertain; inspect history before resending")
		case <-ctx.Done():
			s.rpc.close()
			return fmt.Errorf("turn outcome uncertain: %w", ctx.Err())
		}
	}
}
func (s *session) Interrupt(ctx context.Context) error {
	s.mu.Lock()
	busy, turn := s.busy, s.turnID
	s.mu.Unlock()
	if !busy {
		return nil
	}
	if s.ref.Provider == "Codex" {
		if turn == "" {
			return errors.New("turn identity not yet confirmed; retry stop")
		}
		return s.rpc.call(ctx, "turn/interrupt", map[string]string{"threadId": s.ref.ID, "turnId": turn}, nil)
	}
	return s.rpc.notification("session/cancel", map[string]string{"sessionId": s.ref.ID})
}
func (s *session) Close() error { s.rpc.close(); return nil }

var _ contract.AgentSessionProvider = Provider{}
