package agentplatform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
)

type Config struct {
	HostTools     bool
	BrowserGrants bool
	BrowserActor  string
	BrowserPolicy contract.Policy
	Coordination  bool
	DataDir       string
	Home          string
	Providers     []contract.AgentSessionProvider
	Check         func(context.Context, string, string) error
}
type Server struct {
	searchMu            sync.Mutex
	controls            canonicalControls
	browserDataDirClose func()
	host                *hostModule
	coord               *coordinationModule
	convMu              sync.Mutex
	conversations       map[string]contract.AgentConversation
	ctx                 context.Context
	cancel              context.CancelFunc
	closed              bool
	wg                  sync.WaitGroup
	closeOnce           sync.Once
	selected            map[string]string
	cfg                 Config
	token               string
	mu                  sync.Mutex
	providers           map[string]contract.AgentSessionProvider
	sessions            map[string]*managed
	opening             map[string]bool
	receipts            map[string]receipt
}
type receipt struct {
	Session string `json:"session"`
	Digest  string `json:"digest"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}
type managed struct {
	origin   *controlOrigin
	mu       sync.Mutex
	ready    bool
	done     context.Context
	ref      contract.AgentSessionRef
	client   contract.AgentSession
	busy     bool
	state    string
	sequence uint64
	events   []contract.AgentSessionEvent
	requests map[string]*approval
}
type approval struct {
	contract.AgentSessionRequest
	Digest  string         `json:"digest"`
	Origin  *controlOrigin `json:"origin,omitempty"`
	answer  chan json.RawMessage
	context context.Context
}

func New(ctx context.Context, c Config) (*Server, error) {
	if err := validateBrowserConfig(ctx, c); err != nil {
		return nil, err
	}
	if c.DataDir == "" {
		return nil, errors.New("session service requires a private data directory")
	}
	startupClose, startupErr := lockBrowserDataDir(c)
	if startupErr != nil {
		return nil, startupErr
	}
	keepStartupLock := false
	defer func() {
		if !keepStartupLock && startupClose != nil {
			startupClose()
		}
	}()
	if c.Home == "" {
		c.Home, _ = os.UserHomeDir()
	}
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{ctx: ctx, cancel: cancel, selected: map[string]string{}, cfg: c, providers: map[string]contract.AgentSessionProvider{}, sessions: map[string]*managed{}, opening: map[string]bool{}, receipts: map[string]receipt{}}
	for _, p := range c.Providers {
		if _, ok := s.providers[p.Name()]; ok {
			return nil, errors.New("duplicate session provider")
		}
		s.providers[p.Name()] = p
	}
	if b, err := os.ReadFile(filepath.Join(c.DataDir, "commands.json")); err == nil {
		if err = json.Unmarshal(b, &s.receipts); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for id, r := range s.receipts {
		if r.Status == "running" {
			r.Status = "uncertain"
			r.Error = "service restarted during delivery; inspect native history before sending another command"
			s.receipts[id] = r
		}
	}
	if err := s.loadConversations(); err != nil {
		cancel()
		return nil, err
	}
	if err := s.initCoordination(); err != nil {
		cancel()
		return nil, err
	}
	if err := s.initHostTools(); err != nil {
		cancel()
		return nil, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	s.token = hex.EncodeToString(b)
	incarnation := make([]byte, 32)
	if _, err := rand.Read(incarnation); err != nil {
		return nil, err
	}
	s.controls = canonicalControls{incarnation: hex.EncodeToString(incarnation), receipts: map[string]*controlReceipt{}}
	if c.BrowserGrants {
		adapter, err := s.NewBrowserGrantAdapter(c.BrowserPolicy, c.BrowserActor, filepath.Join(c.DataDir, "browser-receipts.json"))
		if err != nil {
			cancel()
			return nil, err
		}
		if err = s.EnableBrowserGrantRoutes(adapter); err != nil {
			cancel()
			return nil, err
		}
	}
	if err := atomicFile(filepath.Join(c.DataDir, "control-token"), []byte(s.token)); err != nil {
		if s.host != nil && s.host.browser != nil {
			_ = s.host.browser.Close()
		}
		if s.browserDataDirClose != nil {
			s.browserDataDirClose()
		}
		return nil, err
	}
	s.browserDataDirClose = startupClose
	keepStartupLock = true
	return s, nil
}
func atomicFile(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sessions-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}
func (s *Server) TokenFile() string { return filepath.Join(s.cfg.DataDir, "control-token") }
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		owners := make([]*managed, 0, len(s.sessions))
		for _, m := range s.sessions {
			owners = append(owners, m)
		}
		s.mu.Unlock()
		for _, m := range owners {
			m.client.Close()
		}
		s.wg.Wait()
		if s.host != nil && s.host.browser != nil {
			_ = s.host.browser.Close()
		}
		if s.browserDataDirClose != nil {
			s.browserDataDirClose()
		}
	})
}

func (m *managed) Event(kind, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sequence++
	if len(text) > 65536 {
		text = text[:65536]
	}
	m.events = append(m.events, contract.AgentSessionEvent{Sequence: m.sequence, At: time.Now().UTC(), Kind: kind, Text: text})
	if len(m.events) > 512 {
		m.events = m.events[len(m.events)-512:]
	}
}
func (m *managed) Request(ctx context.Context, q contract.AgentSessionRequest) (json.RawMessage, error) {
	m.mu.Lock()
	ready := m.ready
	m.mu.Unlock()
	if !ready {
		return nil, errors.New("approvals are unavailable while session is opening")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	q.ID = hex.EncodeToString(b)
	digest := sha256.Sum256(append([]byte(q.Method), q.Params...))
	a := &approval{AgentSessionRequest: q, Digest: hex.EncodeToString(digest[:]), answer: make(chan json.RawMessage, 1), context: ctx}
	m.mu.Lock()
	if len(m.requests) >= 16 {
		m.mu.Unlock()
		return nil, errors.New("too many pending approvals")
	}
	if m.origin != nil {
		origin := *m.origin
		a.Origin = &origin
	}
	m.requests[q.ID] = a
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.requests, q.ID); m.mu.Unlock() }()
	m.Event("approval", "Waiting for your decision")
	select {
	case result := <-a.answer:
		return result, nil
	case <-m.done.Done():
		return nil, errors.New("session service closed; no permission granted")
	case <-ctx.Done():
		return nil, errors.New("approval expired or connection closed; no permission granted")
	}
}
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		http.Error(w, "loopback host required", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			http.Error(w, "foreign origin", 403)
			return
		}
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(auth), []byte(s.token)) != 1 {
		http.Error(w, "session authentication required", 401)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	act := "read"
	if r.Method == "POST" {
		act = "write"
	}
	if s.cfg.Check != nil {
		if err := s.cfg.Check(r.Context(), "agent/sessions"+r.URL.Path, act); err != nil {
			http.Error(w, "session policy denied", 403)
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/api/host/") {
		s.serveHost(w, r)
		return
	}
	if r.URL.Path == "/api/coordination/mcp" {
		if s.coord.handler == nil {
			http.Error(w, "coordination disabled", 404)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		s.coord.handler.ServeHTTP(w, r)
		return
	}
	result, err := s.route(w, r)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if result == nil {
		result = map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(result)
}
func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1024*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

var routes = map[string]string{"codex-session": "Codex", "claude-session": "Claude", "opencode-session": "OpenCode", "cursor-session": "Cursor", "iugum-session": "Iugum"}

func providerRoute(name string) string {
	for route, p := range routes {
		if p == name {
			return route
		}
	}
	return ""
}
func (s *Server) route(w http.ResponseWriter, r *http.Request) (any, error) {
	if r.Method == "GET" && (r.URL.Path == "/api/session-search" || r.URL.Path == "/api/session-transcript") {
		return s.sessionSearch(r)
	}
	if r.URL.Path == "/api/control-incarnation" && r.Method == "GET" {
		return map[string]string{"incarnation": s.controls.incarnation}, nil
	}
	if r.URL.Path == "/api/conversations" || strings.HasPrefix(r.URL.Path, "/api/conversations/") {
		return s.conversationRoute(r)
	}
	if r.URL.Path == "/api/agents" && r.Method == "GET" {
		out := []map[string]any{}
		for _, name := range []string{"Codex", "Claude", "OpenCode", "Cursor", "Iugum"} {
			if p := s.providers[name]; p != nil {
				out = append(out, map[string]any{"id": providerRoute(name), "name": name, "provider": name, "transport": "sessions", "capabilities": p.Capabilities()})
			}
		}
		return out, nil
	}
	if r.URL.Path == "/api/sessions" && r.Method == "POST" {
		var q contract.AgentSessionCreate
		if err := decode(r, &q); err != nil {
			return nil, err
		}
		return s.createSession(r.Context(), q)
	}
	if r.URL.Path == "/api/sessions" && r.Method == "GET" {
		return s.catalog(r.Context()), nil
	}
	if r.URL.Path == "/api/select" && r.Method == "POST" {
		var ref contract.AgentSessionRef
		if err := decode(r, &ref); err != nil {
			return nil, err
		}
		return s.selectSession(r.Context(), ref)
	}
	if strings.HasPrefix(r.URL.Path, "/api/commands/") && r.Method == "GET" {
		id := strings.TrimPrefix(r.URL.Path, "/api/commands/")
		s.mu.Lock()
		defer s.mu.Unlock()
		receipt, ok := s.receipts[id]
		if !ok {
			return nil, errors.New("unknown command")
		}
		return receipt, nil
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "agents" {
		return nil, errors.New("unknown session route")
	}
	name := routes[parts[2]]
	if name == "" || s.providers[name] == nil {
		return nil, errors.New("unknown or unconfigured provider")
	}
	s.mu.Lock()
	id := r.URL.Query().Get("conversation")
	if id == "" {
		id = s.selected[name]
	}
	m := s.sessions[name+":"+id]
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, errors.New("session service closed")
	}
	if m == nil {
		if r.URL.Query().Get("conversation") != "" {
			return nil, errors.New("unknown session")
		}
		if r.Method != "GET" {
			return nil, errors.New("choose a session from Session Hub first")
		}
		switch parts[3] {
		case "status":
			return map[string]any{"name": name, "metadata": map[string]string{"session": "Choose a session from Session Hub."}}, nil
		case "messages", "approvals", "events", "conversations":
			return []any{}, nil
		case "conversation":
			return map[string]any{"id": "main", "title": name, "can_create": s.providers[name].Capabilities().Create}, nil
		case "models":
			return map[string]any{"options": []any{}, "reason": "Choose a session first."}, nil
		case "settings":
			return map[string]any{"fields": []any{}, "reason": "Choose an existing session. No session is created automatically."}, nil
		}
		return nil, errors.New("unknown session operation")
	}
	if id := r.URL.Query().Get("conversation"); id != "" && id != m.ref.ID {
		return nil, errors.New("session changed; refresh before sending")
	}
	if r.Method == "GET" {
		switch parts[3] {
		case "status":
			m.mu.Lock()
			defer m.mu.Unlock()
			return map[string]any{"name": name, "metadata": map[string]string{"session": m.state}, "busy": m.busy, "capabilities": s.providers[name].Capabilities()}, nil
		case "messages":
			return m.client.History(r.Context())
		case "conversation":
			return map[string]any{"id": m.ref.ID, "title": m.ref.Title, "can_create": s.providers[name].Capabilities().Create}, nil
		case "conversations":
			return []map[string]string{{"id": m.ref.ID, "title": m.ref.Title}}, nil
		case "models":
			return sessionModels(r.Context(), m)
		case "settings":
			return map[string]any{"fields": []any{}, "reason": "Existing session controlled by the shared Go backend. Use one writer at a time; uncertain commands are never replayed automatically."}, nil
		case "approvals":
			m.mu.Lock()
			defer m.mu.Unlock()
			out := []*approval{}
			for _, a := range m.requests {
				out = append(out, a)
			}
			return out, nil
		case "events":
			after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
			m.mu.Lock()
			defer m.mu.Unlock()
			events := []contract.AgentSessionEvent{}
			for _, e := range m.events {
				if e.Sequence > after {
					events = append(events, e)
				}
			}
			return map[string]any{"events": events, "sequence": m.sequence, "gap": len(m.events) > 0 && after+1 < m.events[0].Sequence}, nil
		}
	}
	if r.Method == "POST" {
		if r.URL.Query().Get("conversation") != m.ref.ID {
			return nil, errors.New("explicit conversation ID required")
		}
		switch parts[3] {
		case "models":
			var q struct {
				Model string `json:"model"`
			}
			if err := decode(r, &q); err != nil {
				return nil, err
			}
			return s.selectModel(r.Context(), m, q.Model)
		case "chat":
			return s.prompt(r, m)
		case "interrupt":
			err := m.client.Interrupt(r.Context())
			if err == nil {
				m.Event("state", "Stop requested; waiting for provider confirmation")
			}
			return nil, err
		case "approve":
			var answer struct {
				ID     string          `json:"id"`
				Digest string          `json:"digest"`
				Result json.RawMessage `json:"result"`
			}
			if err := decode(r, &answer); err != nil {
				return nil, err
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			a := m.requests[answer.ID]
			if a == nil || a.context.Err() != nil || answer.Digest != a.Digest {
				return nil, errors.New("approval expired or changed")
			}
			if err := validateAnswer(a.AgentSessionRequest, answer.Result); err != nil {
				return nil, err
			}
			delete(m.requests, answer.ID)
			a.answer <- answer.Result
			return nil, nil
		}
	}
	return nil, errors.New("unsupported session operation")
}
func (s *Server) selectSession(ctx context.Context, ref contract.AgentSessionRef) (any, error) {
	if s.cfg.Check != nil {
		if err := s.cfg.Check(ctx, "agent/sessions/"+ref.Provider+"/"+ref.ID, "select"); err != nil {
			return nil, errors.New("selection policy denied")
		}
	}
	if err := validRef(ref); err != nil {
		return nil, err
	}
	p := s.providers[ref.Provider]
	if p == nil {
		return nil, errors.New("provider not configured")
	}
	if info, err := os.Stat(ref.Cwd); err != nil || !info.IsDir() {
		return nil, errors.New("session working directory does not exist")
	}
	s.mu.Lock()
	key := ref.Provider + ":" + ref.ID
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("session service closed")
	}
	if old := s.sessions[key]; old != nil {
		if old.ref.Cwd != ref.Cwd {
			s.mu.Unlock()
			return nil, errors.New("session working directory changed")
		}
		s.selected[ref.Provider] = ref.ID
		s.mu.Unlock()
		return map[string]string{"agent": providerRoute(ref.Provider)}, nil
	}
	if s.opening[key] {
		s.mu.Unlock()
		return nil, errors.New("session is opening")
	}
	if len(s.sessions)+len(s.opening) >= 32 {
		s.mu.Unlock()
		return nil, errors.New("session owner limit reached (32); restart service to release owners")
	}
	s.opening[key] = true
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	defer func() { s.mu.Lock(); delete(s.opening, key); s.mu.Unlock() }()
	m := &managed{ref: ref, state: "Opening", done: s.ctx, requests: map[string]*approval{}, events: []contract.AgentSessionEvent{}}
	client, err := p.Open(s.ctx, ref, m)
	if err != nil {
		if client != nil {
			client.Close()
		}
		return nil, err
	}
	if client == nil {
		return nil, errors.New("provider returned no session")
	}
	m.client = client
	if err = s.restoreModel(ctx, m); err != nil {
		client.Close()
		return nil, err
	}
	s.mu.Lock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		client.Close()
		return nil, errors.New("session service closed")
	}
	m.mu.Lock()
	m.state = "Ready"
	m.ready = true
	m.mu.Unlock()
	s.sessions[key] = m
	s.selected[ref.Provider] = ref.ID
	s.mu.Unlock()
	m.Event("state", "Ready")
	return map[string]string{"agent": providerRoute(ref.Provider)}, nil
}

func (s *Server) prompt(r *http.Request, m *managed) (any, error) {
	s.convMu.Lock()
	convLocked := true
	defer func() {
		if convLocked {
			s.convMu.Unlock()
		}
	}()
	linked := false
	for _, c := range s.conversations {
		for _, ref := range c.Links {
			if sameSession(ref, m.ref) {
				linked = true
			}
		}
	}
	if linked {
		return nil, errors.New("session belongs to an Iugum conversation; send through its conversation route")
	}

	var q struct {
		Text      string `json:"text"`
		CommandID string `json:"commandId"`
	}
	if err := decode(r, &q); err != nil {
		return nil, err
	}
	if !uuid.MatchString(q.CommandID) || strings.TrimSpace(q.Text) == "" || len(q.Text) > 200000 {
		return nil, errors.New("a valid command ID and prompt are required")
	}
	for _, c := range s.conversations {
		for _, cmd := range c.Commands {
			if cmd.ID == q.CommandID {
				return nil, errors.New("command belongs to an Iugum conversation; not replayed")
			}
		}
	}
	key := m.ref.Provider + ":" + m.ref.ID
	hash := sha256.Sum256([]byte(q.Text))
	digest := hex.EncodeToString(hash[:])
	s.mu.Lock()
	if old, ok := s.receipts[q.CommandID]; ok {
		s.mu.Unlock()
		if old.Session != key || old.Digest != digest {
			return nil, errors.New("command ID belongs to a different request")
		}
		return old, errors.New("command already recorded as " + old.Status + "; inspect history; not replayed")
	}
	if s.closed || s.ctx.Err() != nil || s.sessions[key] != m {
		s.mu.Unlock()
		return nil, errors.New("session is changing; refresh")
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		s.mu.Unlock()
		return nil, errors.New("session already working")
	}
	m.busy = true
	m.state = "Working"
	m.mu.Unlock()
	s.receipts[q.CommandID] = receipt{Session: key, Digest: digest, Status: "running"}
	err := s.saveReceipts()
	if err == nil {
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if err != nil {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
		return nil, err
	}
	s.convMu.Unlock()
	convLocked = false
	finished := make(chan error, 1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		err := m.client.Prompt(ctx, q.Text)
		status := "complete"
		state := "Ready"
		detail := ""
		if err != nil {
			status = "uncertain"
			state = "Inspect history before resending"
			detail = err.Error()
		}
		m.mu.Lock()
		m.busy = false
		m.state = state
		m.mu.Unlock()
		m.Event("state", state)
		s.mu.Lock()
		entry := s.receipts[q.CommandID]
		entry.Status = status
		entry.Error = detail
		s.receipts[q.CommandID] = entry
		saveErr := s.saveReceipts()
		s.mu.Unlock()
		if err == nil {
			err = saveErr
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		return map[string]string{"commandId": q.CommandID}, err
	case <-r.Context().Done():
		return nil, r.Context().Err()
	}
}
func (s *Server) saveReceipts() error {
	b, err := json.Marshal(s.receipts)
	if err != nil {
		return err
	}
	return atomicFile(filepath.Join(s.cfg.DataDir, "commands.json"), b)
}
func validateAnswer(q contract.AgentSessionRequest, result json.RawMessage) error {
	if q.Method == "can_use_tool" || q.Method == "permission.asked" || q.Method == "question.asked" {
		return validateNativeAnswer(q, result)
	}
	var value map[string]any
	if json.Unmarshal(result, &value) != nil {
		return errors.New("invalid approval answer")
	}
	if q.Method == "session/request_permission" {
		var p struct {
			Options []struct {
				ID   string `json:"optionId"`
				Kind string `json:"kind"`
			} `json:"options"`
		}
		_ = json.Unmarshal(q.Params, &p)
		outcome, _ := value["outcome"].(map[string]any)
		if outcome["outcome"] == "cancelled" {
			return nil
		}
		if outcome["outcome"] == "selected" {
			for _, o := range p.Options {
				if o.ID == outcome["optionId"] && (o.Kind == "allow_once" || o.Kind == "reject_once") {
					return nil
				}
			}
		}
		return errors.New("answer must select a listed one-time permission or cancel")
	}
	if strings.HasSuffix(q.Method, "requestApproval") {
		decision, _ := value["decision"].(string)
		if decision == "accept" || decision == "decline" || decision == "cancel" {
			var params struct {
				Available []json.RawMessage `json:"availableDecisions"`
			}
			if err := json.Unmarshal(q.Params, &params); err != nil && len(q.Params) > 0 {
				return errors.New("invalid approval request")
			}
			if params.Available != nil {
				for _, offered := range params.Available {
					var choice string
					if json.Unmarshal(offered, &choice) == nil && choice == decision {
						return nil
					}
				}
				return errors.New("decision was not offered")
			}
			return nil
		}
		return errors.New("invalid approval decision")
	}
	if q.Method == "cursor/create_plan" {
		outcome, _ := value["outcome"].(map[string]any)
		if outcome["outcome"] == "accepted" || outcome["outcome"] == "rejected" || outcome["outcome"] == "cancelled" {
			return nil
		}
		return errors.New("invalid plan decision")
	}
	if q.Method == "cursor/ask_question" {
		return validateCursorQuestion(q, result)
	}
	if q.Method == "item/tool/requestUserInput" {
		return validateCodexQuestions(q, result)
	}
	return fmt.Errorf("unsupported approval response")
}
