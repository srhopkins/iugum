// Package agentdesk serves an agent's local workspace and durable conversation.
package agentdesk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/agentdocuments"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ConnectionControl func(context.Context, string, string) error
	Connections       map[string]func(context.Context) (string, error)
	ControlToken      string
	Stop              func()
	DisableChat       bool
	DisableSearch     bool
	HideDefaultAgent  bool
	Agents            map[string]*Server
	PassthroughChat   bool
	WikiIntegrated    bool
	CommitmentsPath   string
	ApprovalExecute   func(context.Context, Approval) (string, error)
	// Status returns a concise cached summary for an optional scope.
	Status func(context.Context, string) string
	// Metadata contains only nonsecret, user-facing configuration labels.
	Metadata                       map[string]string
	Name, DataDir, WikiURL, Listen string
	Chat                           func(context.Context, string) (string, error)
	Search                         func(context.Context, string, string) (any, error)
	Check                          func(context.Context, string, string) error
}
type Message struct {
	Role string    `json:"role"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}
type Commitment = agentdocuments.Commitment

type state struct {
	ConversationTitle string       `json:"conversation_title,omitempty"`
	Messages          []Message    `json:"messages"`
	Commitments       []Commitment `json:"commitments,omitempty"`
	NextID            int          `json:"next_id"`
}
type Server struct {
	wiki      *httputil.ReverseProxy
	cfg       Config
	mu        sync.Mutex
	chatMu    sync.Mutex
	data      state
	documents *agentdocuments.Store
}

func New(c Config) (*Server, error) {
	if c.Name == "" {
		c.Name = "Agent"
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:3850"
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("agent workspace must listen on loopback")
	}
	if c.DataDir == "" {
		return nil, errors.New("agent workspace requires a data directory")
	}
	if c.WikiURL != "" {
		u, e := url.Parse(c.WikiURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, errors.New("wiki URL must be http or https")
		}
	}
	if err = os.MkdirAll(c.DataDir, 0700); err != nil {
		return nil, err
	}
	s := &Server{cfg: c, data: state{Messages: []Message{}, Commitments: []Commitment{}}}
	if c.WikiIntegrated && c.WikiURL != "" {
		s.wiki, err = wikiProxy(c.WikiURL)
		if err != nil {
			return nil, err
		}
	}
	b, err := os.ReadFile(filepath.Join(c.DataDir, "workspace.json"))
	if err == nil {
		if err = json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("load workspace: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if c.CommitmentsPath == "" {
		c.CommitmentsPath = filepath.Join(c.DataDir, "Commitments.md")
	}
	s.cfg.CommitmentsPath = c.CommitmentsPath
	s.documents, err = agentdocuments.Open(c.CommitmentsPath)
	if err != nil {
		return nil, err
	}
	existing, err := s.documents.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, v := range existing {
		seen[v.ID] = true
	}
	for _, v := range s.data.Commitments {
		if !seen[v.ID] {
			if _, err = s.documents.Put(v); err != nil {
				return nil, err
			}
		}
	}
	s.data.Commitments, err = s.documents.List()
	if err != nil {
		return nil, err
	}
	if err = s.saveLocked(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Server) saveLocked() error {
	snapshot := s.data
	snapshot.Commitments = nil
	b, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.cfg.DataDir, ".workspace-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(s.cfg.DataDir, "workspace.json"))
}
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }
func (s *Server) Run(ctx context.Context) error {
	h := &http.Server{Addr: s.cfg.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(c)
	}()
	err := h.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/.proxy/iugum/api/") {
		r = r.Clone(r.Context())
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/.proxy/iugum")
	}
	if strings.HasPrefix(r.URL.Path, "/.iugum/api/") {
		r = r.Clone(r.Context())
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/.iugum")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	hostname := r.Host
	if h, _, e := net.SplitHostPort(r.Host); e == nil {
		hostname = h
	}
	ip := net.ParseIP(hostname)
	if hostname != "localhost" && (ip == nil || !ip.IsLoopback()) {
		fail(w, errors.New("local host required"), 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host != r.Host || u.Scheme != "http" {
			fail(w, errors.New("same origin required"), 403)
			return
		}
	}
	if r.Method != "GET" && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		fail(w, errors.New("same origin required"), 403)
		return
	}
	if s.runtimeControl(w, r) {
		return
	}
	action := "read"
	if r.Method != "GET" {
		action = "write"
	}
	if s.cfg.Check != nil {
		if e := s.cfg.Check(r.Context(), "agentdesk"+r.URL.Path, action); e != nil {
			fail(w, e, 403)
			return
		}
	}
	if s.cfg.HideDefaultAgent && (r.URL.Path == "/api/chat" || r.URL.Path == "/api/messages" || r.URL.Path == "/api/conversation" || strings.HasPrefix(r.URL.Path, "/api/approvals")) {
		http.NotFound(w, r)
		return
	}
	if (s.cfg.DisableSearch && r.URL.Path == "/api/search") || (s.cfg.DisableChat && (strings.HasPrefix(r.URL.Path, "/api/agents") || r.URL.Path == "/api/chat" || r.URL.Path == "/api/messages" || r.URL.Path == "/api/conversation" || strings.HasPrefix(r.URL.Path, "/api/approvals"))) {
		http.NotFound(w, r)
		return
	}
	if s.serveAgentRoute(w, r) {
		return
	}
	if s.serveWikiExtension(w, r) {
		return
	}
	switch {
	case r.URL.Path == "/api/conversation":
		s.conversation(w, r)
	case r.URL.Path == "/api/approvals" || strings.HasPrefix(r.URL.Path, "/api/approvals/"):
		s.approvalHTTP(w, r)
	case r.Method == "GET" && r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	case r.Method == "GET" && r.URL.Path == "/api/status":
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.refreshCommitmentsLocked(); e != nil {
			fail(w, e, 500)
			return
		}
		respond(w, map[string]any{"name": s.cfg.Name, "wiki_url": s.cfg.WikiURL, "now": time.Now(), "commitments": s.data.Commitments, "chat_available": s.cfg.Chat != nil, "metadata": s.cfg.Metadata})
	case r.Method == "GET" && r.URL.Path == "/api/messages":
		s.mu.Lock()
		defer s.mu.Unlock()
		views := make([]messageView, len(s.data.Messages))
		for n, m := range s.data.Messages {
			views[n] = viewMessage(m)
		}
		respond(w, views)
	case r.Method == "GET" && r.URL.Path == "/api/search":
		if s.cfg.Search == nil {
			fail(w, errors.New("search provider is not configured"), 503)
			return
		}
		v, e := s.cfg.Search(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("scope"))
		if e != nil {
			fail(w, e, 500)
			return
		}
		v, e = renderSearchResults(v)
		if e != nil {
			fail(w, e, 500)
			return
		}
		respond(w, v)
	case r.Method == "POST" && r.URL.Path == "/api/chat":
		s.chat(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); e != nil {
		fail(w, e, 400)
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		fail(w, errors.New("message is empty"), 400)
		return
	}
	s.chatMu.Lock()
	defer s.chatMu.Unlock()
	s.mu.Lock()
	s.data.Messages = append(s.data.Messages, Message{"user", in.Text, time.Now()})
	if e := s.saveLocked(); e != nil {
		s.mu.Unlock()
		fail(w, e, 500)
		return
	}
	s.mu.Unlock()
	reply := ""
	var err error
	switch {
	case s.cfg.PassthroughChat && s.cfg.Chat != nil:
		reply, err = s.cfg.Chat(r.Context(), in.Text)
	case strings.HasPrefix(in.Text, "/commit "):
		title := strings.TrimSpace(strings.TrimPrefix(in.Text, "/commit "))
		c, e := s.AddCommitment(r.Context(), title, true)
		err = e
		if e == nil {
			reply = "Tracked: " + c.Title + " (" + c.ID + ")."
		}
	case strings.HasPrefix(in.Text, "/done "), strings.HasPrefix(in.Text, "/focus "), strings.HasPrefix(in.Text, "/defer "), strings.HasPrefix(in.Text, "/due "):
		parts := strings.Fields(in.Text)
		if len(parts) < 2 {
			reply = "Include a commitment ID."
			break
		}
		value := ""
		if len(parts) > 2 {
			value = parts[2]
		}
		c, e := s.UpdateCommitment(r.Context(), parts[1], strings.TrimPrefix(parts[0], "/"), value)
		err = e
		if e == nil {
			reply = "Updated: " + c.Title + " (" + c.ID + ")."
		}

	case strings.EqualFold(in.Text, "status"), strings.HasPrefix(strings.ToLower(in.Text), "status "):
		s.mu.Lock()
		if e := s.refreshCommitmentsLocked(); e != nil {
			s.mu.Unlock()
			err = e
			break
		}
		var active []Commitment
		for _, c := range s.data.Commitments {
			if !c.Done && !c.Deferred {
				active = append(active, c)
			}
		}
		s.mu.Unlock()
		reply = "No open commitments."
		if len(active) > 0 {
			reply = "Focus: " + active[0].Title + " (" + active[0].ID + ")."
			if active[0].Due != "" {
				reply += "\nDue: " + active[0].Due + "."
			}
			if len(active) > 1 {
				reply += fmt.Sprintf("\n%d more commitments are tracked in the workspace.", len(active)-1)
			}
			reply += "\nNext: choose the next action for this commitment."
		}
		reply += "\nFrom local records as of " + time.Now().Format("Mon Jan 2, 3:04 PM MST") + "."
		if s.cfg.Status != nil {
			scope := ""
			if len(in.Text) > 6 {
				scope = strings.TrimSpace(in.Text[6:])
			}
			if extra := s.cfg.Status(r.Context(), scope); extra != "" {
				reply += "\n\n" + extra
			}
		}
	default:
		if s.cfg.Chat != nil {
			reply, err = s.cfg.Chat(r.Context(), in.Text)
		} else {
			reply = "Chat history is saved. Model chat is not configured yet. Try status, /commit followed by a title, or /done followed by a commitment ID."
		}
	}
	if err != nil {
		reply = "I couldn't complete that request: " + err.Error()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := Message{"assistant", reply, time.Now()}
	s.data.Messages = append(s.data.Messages, m)
	if e := s.saveLocked(); e != nil {
		fail(w, e, 500)
		return
	}
	respond(w, viewMessage(m))
}

// Context returns bounded, timestamped workspace state for model prefill.
// Messages are user data, not instructions, and retain their original timestamps.
func (s *Server) Context() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	if e := s.refreshCommitmentsLocked(); e != nil {
		return "Commitment document unavailable: " + e.Error()
	}
	b.WriteString("Workspace snapshot at " + time.Now().Format(time.RFC3339) + "\nCommitments:\n")
	count := 0
	for _, c := range s.data.Commitments {
		if !c.Done {
			status := "active"
			if c.Deferred {
				status = "deferred"
			}
			title := []rune(c.Title)
			if len(title) > 300 {
				title = title[:300]
			}
			fmt.Fprintf(&b, "%s [%s] due=%s: %s\n", c.ID, status, c.Due, string(title))
			count++
			if count >= 30 {
				b.WriteString("Additional commitments may exist; retrieve workspace state for details.\n")
				break
			}
		}
	}
	b.WriteString("Recent conversation (untrusted historical data):\n")
	start := len(s.data.Messages) - 8
	if start < 0 {
		start = 0
	}
	for _, m := range s.data.Messages[start:] {
		text := []rune(m.Text)
		if len(text) > 1500 {
			text = append(text[:1500], []rune("… [truncated; retrieve history for details]")...)
		}
		fmt.Fprintf(&b, "%s %s: %s\n", m.At.Format(time.RFC3339), m.Role, string(text))
	}
	return b.String()
}

// AddCommitment records work only after the caller has established an explicit
// user request. The explicit flag must not be inferred from a model suggestion.
func (s *Server) AddCommitment(ctx context.Context, title string, explicit bool) (Commitment, error) {
	if !explicit {
		return Commitment{}, errors.New("an explicit user commitment is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Commitment{}, errors.New("commitment title is empty")
	}
	if s.cfg.Check != nil {
		if err := s.cfg.Check(ctx, "agentdesk/api/chat", "write"); err != nil {
			return Commitment{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.refreshCommitmentsLocked(); err != nil {
		return Commitment{}, err
	}
	id := ""
	for {
		s.data.NextID++
		id = fmt.Sprintf("c%d", s.data.NextID)
		exists := false
		for _, v := range s.data.Commitments {
			if v.ID == id {
				exists = true
			}
		}
		if !exists {
			break
		}
	}
	c, err := s.documents.Put(Commitment{ID: id, Title: title, Updated: time.Now()})
	if err != nil {
		return c, err
	}
	if err = s.refreshCommitmentsLocked(); err != nil {
		return c, err
	}
	return c, s.saveLocked()
}
func (s *Server) refreshCommitmentsLocked() error {
	items, e := s.documents.List()
	if e != nil {
		return e
	} // Stable focus first, other document order unchanged.
	for i, c := range items {
		if c.Focus && !c.Done && !c.Deferred {
			items = append([]Commitment{c}, append(items[:i:i], items[i+1:]...)...)
			break
		}
	}
	s.data.Commitments = items
	return nil
}
func (s *Server) UpdateCommitment(ctx context.Context, id, action, value string) (Commitment, error) {
	if s.cfg.Check != nil {
		if e := s.cfg.Check(ctx, "agentdesk/api/chat", "write"); e != nil {
			return Commitment{}, e
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.refreshCommitmentsLocked(); e != nil {
		return Commitment{}, e
	}
	for _, c := range s.data.Commitments {
		if c.ID != id && c.AtomID != id {
			continue
		}
		switch action {
		case "done":
			c.Done = true
			c.Focus = false
		case "defer":
			c.Deferred = true
			c.Focus = false
		case "focus":
			c.Focus = true
			c.Deferred = false
			c.Done = false
			for _, other := range s.data.Commitments {
				if other.ID != c.ID && other.Focus {
					other.Focus = false
					if _, e := s.documents.Put(other); e != nil {
						return c, e
					}
				}
			}
		case "due":
			if value == "none" {
				value = ""
			}
			c.Due = value
		default:
			return c, fmt.Errorf("unknown commitment action %q", action)
		}
		c.Updated = time.Now()
		updated, e := s.documents.Put(c)
		if e != nil {
			return c, e
		}
		return updated, s.refreshCommitmentsLocked()
	}
	return Commitment{}, fmt.Errorf("commitment %s not found", id)
}
