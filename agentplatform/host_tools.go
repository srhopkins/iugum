package agentplatform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Host tools control one explicitly registered extension-host window. They do
// not infer a browser tab or gain access to the workbench DOM or mouse.
type HostRegistration struct {
	Instance     string   `json:"instance"`
	Label        string   `json:"label"`
	Capabilities []string `json:"capabilities"`
}
type HostIdentity struct {
	Instance string `json:"instance"`
	Lease    string `json:"lease"`
}
type HostCall struct {
	Instance  string          `json:"instance"`
	CommandID string          `json:"commandId"`
	Operation string          `json:"operation"`
	Args      json.RawMessage `json:"args"`
}
type HostCommand struct {
	HostCall
	State   string          `json:"state"`
	Expires time.Time       `json:"expires"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}
type HostResult struct {
	HostIdentity
	CommandID string          `json:"commandId"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}
type hostWindow struct {
	HostRegistration
	Lease   string
	Expires time.Time
}
type hostModule struct {
	mu       sync.Mutex
	windows  map[string]hostWindow
	commands map[string]*HostCommand
	pairings map[string]hostPairPending
	handler  http.Handler
	browser  *BrowserGrantAdapter
}

func (s *Server) initHostTools() error {
	if !s.cfg.HostTools {
		return nil
	}
	if s.cfg.Check == nil {
		return errors.New("editor host tools require actual application policy")
	}
	h := &hostModule{windows: map[string]hostWindow{}, commands: map[string]*HostCommand{}, pairings: map[string]hostPairPending{}}
	s.host = h
	b, e := os.ReadFile(filepath.Join(s.cfg.DataDir, "host-commands.json"))
	if e == nil {
		if e = json.Unmarshal(b, &h.commands); e != nil {
			return e
		}
		if h.commands == nil {
			return errors.New("invalid editor command store")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	for id, c := range h.commands {
		if c == nil || c.CommandID != id || !uuid.MatchString(id) || !uuid.MatchString(c.Instance) {
			return errors.New("invalid editor command receipt")
		}
		if c.State == "queued" || c.State == "dispatched" {
			c.State = "uncertain"
			c.Error = "service restarted; editor action is not replayed"
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "iugum-editor", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ffai_workers", Description: "List FFAI worker process metadata in the exact named ffai-agent container as abc using clean Bash. Bounded read-only discovery; no arguments, transcripts or shell commands. Results are unmanaged tmux panes, not controllable Iugum session IDs. Claude process presence does not establish idle state or authorization to send."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		if e := s.hostCheck(ctx, "ffai_workers", "read"); e != nil {
			return nil, nil, e
		}
		return nil, listFFAIWorkers(ctx), nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "editor_windows", Description: "List exact live extension-host windows and capabilities. Does not list all browser tabs. Mouse unsupported without an actual client connector."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		v, e := s.hostWindows(ctx)
		return nil, v, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "editor_call", InputSchema: editorCallSchema(), Description: "For Beads navigation prefer editor_open_beads, with all requested navigation options together in one flat call. This generic tool remains available for compatibility. Queue an explicit typed editor operation at an exact instance using a fresh commandId. instance must be the exact UUID from editor_windows. commandId must be a fresh UUID string (8-4-4-4-12 hex digits), never a label. open_beads accepts optional typed view/filter/search/sort/lanes/ticket navigation at the configured Beads origin; args:{} opens the configured landing view. Receipt confirms execution, not visible rendering; verify the owning browser. If input validation rejects a call before queuing, correct the fields in this turn; do not query status for the rejected ID. Operations inspect, open_file, focus, open_beads. No arbitrary commands or mouse. Returns receipt; use editor_status for completion. Never retry uncertain actions." + ` Correct JSON structure for opening Beads: {"instance":"UUID from editor_windows","commandId":"fresh UUID","operation":"open_beads","args":{}}. operation belongs at the top level. args contains only navigation options, never another args or operation field. Keep args empty to open the configured landing view.`}, func(ctx context.Context, _ *mcp.CallToolRequest, q struct {
		Instance  string         `json:"instance"`
		CommandID string         `json:"commandId"`
		Operation string         `json:"operation"`
		Args      map[string]any `json:"args"`
	}) (*mcp.CallToolResult, any, error) {
		args, e := json.Marshal(q.Args)
		if e != nil {
			return nil, nil, e
		}
		v, e := s.hostCall(ctx, HostCall{Instance: q.Instance, CommandID: q.CommandID, Operation: q.Operation, Args: args})
		return nil, v, e
	})
	mcp.AddTool(server, &mcp.Tool{Name: "editor_open_beads", InputSchema: editorOpenBeadsSchema(), Description: "Open the configured Beads viewer in this exact editor instance. Prefer this dedicated tool for Beads. All fields are top level: instance, fresh commandId UUID, view, projects, statuses, labels, search, priorities, types, assignees, sort, lanes, ticket, ticketProject. projects/statuses/labels/priorities/types/assignees are arrays of strings. Use view=board for a Kanban board. Include every requested filter and ticket together in one call; do not issue a second generic call that drops options. ticket opens its drawer while retaining the supplied board filters. Omit all optional options for the configured landing view. It accepts no args wrapper and no operation field. Use editor_status with the returned exact receipt commandId to verify completion. Receipt confirms execution, not visible rendering; verify the owning browser. Never replay queued, dispatched, or uncertain actions."}, func(ctx context.Context, _ *mcp.CallToolRequest, q map[string]any) (*mcp.CallToolResult, any, error) {
		return s.editorOpenBeads(ctx, q)
	})
	mcp.AddTool(server, &mcp.Tool{Name: "editor_status", InputSchema: editorStatusSchema(), Description: "Read the original editor action receipt using the exact instance and commandId returned by a queued call. Never look up a rejected or invented ID. A missing receipt does not mean the operation is unsupported. Uncertain means inspect before issuing another action."}, func(ctx context.Context, _ *mcp.CallToolRequest, q struct {
		Instance  string `json:"instance"`
		CommandID string `json:"commandId"`
	}) (*mcp.CallToolResult, any, error) {
		v, e := s.hostStatus(ctx, q.Instance, q.CommandID)
		return nil, v, e
	})
	h.handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return s.saveHost()
}
func (s *Server) hostCheck(ctx context.Context, op, act string) error {
	if s.host == nil {
		return errors.New("editor host tools disabled")
	}
	if s.cfg.Check != nil {
		return s.cfg.Check(ctx, "agent/host/"+op, act)
	}
	return nil
}
func (s *Server) saveHost() error {
	b, e := json.Marshal(s.host.commands)
	if e != nil {
		return e
	}
	return atomicFile(filepath.Join(s.cfg.DataDir, "host-commands.json"), b)
}
func hostOperation(op string) bool {
	return op == "inspect" || op == "open_file" || op == "focus" || op == "open_beads"
}
func (s *Server) hostRegister(q HostRegistration) (HostIdentity, error) {
	return s.hostRegisterContext(context.Background(), q)
}
func (s *Server) hostRegisterContext(ctx context.Context, q HostRegistration) (HostIdentity, error) {
	if e := s.hostCheck(ctx, "register", "write"); e != nil {
		return HostIdentity{}, e
	}
	if !uuid.MatchString(q.Instance) || len(q.Label) > 120 || len(q.Capabilities) > 4 {
		return HostIdentity{}, errors.New("invalid editor registration")
	}
	seen := map[string]bool{}
	for _, v := range q.Capabilities {
		if !hostOperation(v) || seen[v] {
			return HostIdentity{}, errors.New("unsupported editor capability")
		}
		seen[v] = true
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.windows[q.Instance]; ok && time.Now().Before(old.Expires) {
		return HostIdentity{}, errors.New("editor instance already registered")
	}
	for _, c := range h.commands {
		if c.Instance == q.Instance && (c.State == "queued" || c.State == "dispatched") {
			c.State = "uncertain"
			c.Error = "editor registration changed; action not replayed"
		}
	}
	if e := s.saveHost(); e != nil {
		return HostIdentity{}, e
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return HostIdentity{}, e
	}
	lease := hex.EncodeToString(b)
	h.windows[q.Instance] = hostWindow{q, lease, time.Now().Add(time.Minute)}
	return HostIdentity{q.Instance, lease}, nil
}
func (s *Server) hostWindows(ctx context.Context) (any, error) {
	if e := s.hostCheck(ctx, "windows", "read"); e != nil {
		return nil, e
	}
	s.host.mu.Lock()
	defer s.host.mu.Unlock()
	out := []HostRegistration{}
	for _, w := range s.host.windows {
		if time.Now().Before(w.Expires) {
			out = append(out, w.HostRegistration)
		}
	}
	return map[string]any{"windows": out, "scope": "extension-host-window", "mouse": false, "mouseReason": "No connector bound to actual browser client"}, nil
}
func validateHostCall(q HostCall) error {
	if len(q.Args) > 16*1024 {
		return errors.New("editor arguments too large")
	}
	if !uuid.MatchString(q.Instance) {
		return errors.New("invalid editor instance: use the exact live UUID from editor_windows; this call was not queued")
	}
	if !uuid.MatchString(q.CommandID) {
		return errors.New("invalid editor commandId: generate a fresh UUID in 8-4-4-4-12 hexadecimal form, not a word label; this call was not queued; correct the ID before requesting a receipt")
	}
	if !hostOperation(q.Operation) {
		return errors.New("invalid editor operation: select an advertised inspect, open_file, focus, or open_beads capability; this call was not queued")
	}
	var a struct {
		Path      string `json:"path"`
		Line      int    `json:"line"`
		Column    int    `json:"column"`
		EndLine   int    `json:"endLine"`
		EndColumn int    `json:"endColumn"`
		Group     int    `json:"group"`
		Pane      string `json:"pane"`
	}
	if len(q.Args) == 0 {
		q.Args = json.RawMessage(`{}`)
	}
	var keys map[string]json.RawMessage
	if e := json.Unmarshal(q.Args, &keys); e != nil || keys == nil {
		return errors.New("editor args must be an object")
	}
	if q.Operation == "open_beads" {
		return validateBeadsNavigation(q.Args)
	}
	for key, value := range keys {
		if string(value) == "null" {
			return errors.New("editor operation fields cannot be null; this call was not queued")
		}
		allowed := false
		switch q.Operation {
		case "open_file":
			switch key {
			case "path", "line", "column", "endLine", "endColumn", "group":
				allowed = true
			}
		case "focus":
			allowed = key == "pane"
		}
		if !allowed {
			return errors.New("argument does not belong to editor operation")
		}
	}
	d := json.NewDecoder(strings.NewReader(string(q.Args)))
	d.DisallowUnknownFields()
	if e := d.Decode(&a); e != nil {
		return e
	}
	if q.Operation == "open_file" {
		if a.Path == "" || len(a.Path) > 4096 || filepath.IsAbs(a.Path) || strings.Contains(a.Path, "\\") || strings.ContainsRune(a.Path, 0) || a.Path == ".." || strings.HasPrefix(filepath.Clean(a.Path), "../") || strings.Contains("/"+a.Path+"/", "/../") || a.Line < 1 || a.Column < 0 || a.EndLine < 0 || a.EndColumn < 0 || a.Group < 1 || a.Group > 9 {
			return errors.New("invalid workspace file position or group")
		}
	}
	if q.Operation == "open_file" {
		const maxSafeInteger = 9007199254740991
		for _, value := range []int{a.Line, a.Column, a.EndLine, a.EndColumn, a.Group} {
			if value > maxSafeInteger {
				return errors.New("editor position exceeds safe integer range")
			}
		}
		endLine, startColumn, endColumn := a.EndLine, a.Column, a.EndColumn
		if endLine == 0 {
			endLine = a.Line
		}
		if startColumn == 0 {
			startColumn = 1
		}
		if endColumn == 0 {
			if a.EndLine != 0 {
				endColumn = 1
			} else {
				endColumn = startColumn
			}
		}
		if endLine < a.Line || endLine == a.Line && endColumn < startColumn {
			return errors.New("reversed editor selection; this call was not queued")
		}
	}
	if q.Operation == "focus" {
		switch a.Pane {
		case "editor", "sidebar", "panel", "terminal", "chat":
		default:
			return errors.New("unsupported editor pane")
		}
	}
	return nil
}
func (s *Server) hostCall(ctx context.Context, q HostCall) (HostCommand, error) {
	if e := s.hostCheck(ctx, q.Operation, "write"); e != nil {
		return HostCommand{}, e
	}
	if e := validateHostCall(q); e != nil {
		return HostCommand{}, e
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.commands[q.CommandID]; ok {
		return HostCommand{}, errors.New("editor commandId already reserved")
	}
	w, ok := h.windows[q.Instance]
	if !ok || time.Now().After(w.Expires) {
		return HostCommand{}, errors.New("editor instance disconnected")
	}
	supported := false
	for _, c := range w.Capabilities {
		supported = supported || c == q.Operation
	}
	if !supported {
		return HostCommand{}, errors.New("editor capability unavailable")
	}
	pending := 0
	for _, c := range h.commands {
		if c.Instance == q.Instance && (c.State == "queued" || c.State == "dispatched") && time.Now().Before(c.Expires) {
			pending++
		}
	}
	if pending >= 32 {
		return HostCommand{}, errors.New("editor queue full")
	}
	c := &HostCommand{HostCall: q, State: "queued", Expires: time.Now().Add(30 * time.Second)}
	h.commands[q.CommandID] = c
	if e := s.saveHost(); e != nil {
		delete(h.commands, q.CommandID)
		return HostCommand{}, e
	}
	return *c, nil
}
func (h *hostModule) window(q HostIdentity) (hostWindow, error) {
	w, ok := h.windows[q.Instance]
	if !ok || w.Lease != q.Lease || time.Now().After(w.Expires) {
		return w, errors.New("stale editor identity")
	}
	return w, nil
}
func (s *Server) hostPoll(ctx context.Context, q HostIdentity) (*HostCommand, error) {
	if e := s.hostCheck(ctx, "poll", "write"); e != nil {
		return nil, e
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	w, e := h.window(q)
	if e != nil {
		return nil, e
	}
	w.Expires = time.Now().Add(time.Minute)
	h.windows[q.Instance] = w
	for _, c := range h.commands {
		if c.Instance != q.Instance || c.State != "queued" {
			continue
		}
		if time.Now().After(c.Expires) {
			c.State = "uncertain"
			c.Error = "editor action expired; not replayed"
			if e = s.saveHost(); e != nil {
				return nil, e
			}
			continue
		}
		if e = s.hostCheck(ctx, c.Operation, "write"); e != nil {
			return nil, e
		}
		c.State = "dispatched"
		if e = s.saveHost(); e != nil {
			c.State = "uncertain"
			return nil, e
		}
		copy := *c
		return &copy, nil
	}
	return nil, nil
}
func (s *Server) hostResult(q HostResult) error { return s.hostResultContext(context.Background(), q) }
func (s *Server) hostResultContext(ctx context.Context, q HostResult) error {
	if e := s.hostCheck(ctx, "result", "write"); e != nil {
		return e
	}
	if len(q.Result) > 64*1024 || len(q.Error) > 2000 {
		return errors.New("editor result too large")
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, e := h.window(q.HostIdentity); e != nil {
		return e
	}
	c := h.commands[q.CommandID]
	if c == nil || c.Instance != q.Instance || c.State != "dispatched" {
		return errors.New("wrong or completed editor command")
	}
	if time.Now().After(c.Expires) {
		c.State = "uncertain"
		c.Error = "editor result arrived after expiry"
	} else {
		c.State = "complete"
		c.Result = q.Result
		c.Error = q.Error
		if q.Error != "" {
			c.State = "failed"
		}
	}
	if e := s.saveHost(); e != nil {
		c.State = "uncertain"
		c.Result = nil
		c.Error = "editor result persistence failed; inspect before another action"
		return e
	}
	return nil
}
func (s *Server) hostStatus(ctx context.Context, instance, id string) (HostCommand, error) {
	if e := s.hostCheck(ctx, "status", "read"); e != nil {
		return HostCommand{}, e
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.commands[id]
	if c == nil || c.Instance != instance {
		return HostCommand{}, errors.New("editor command not found for this instance and commandId; use the exact ID from a queued receipt, not a rejected or invented ID; a missing receipt does not mean an editor capability is unsupported")
	}
	if (c.State == "queued" || c.State == "dispatched") && time.Now().After(c.Expires) {
		c.State = "uncertain"
		c.Error = "editor action expired; inspect before another action"
		if e := s.saveHost(); e != nil {
			return HostCommand{}, e
		}
	}
	return *c, nil
}
func (s *Server) serveHost(w http.ResponseWriter, r *http.Request) {
	if s.host == nil {
		http.Error(w, "editor host tools disabled", 404)
		return
	}
	if e := s.hostCheck(r.Context(), "transport", "write"); e != nil {
		http.Error(w, "editor policy denied", 403)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/host/browser/") {
		s.host.mu.Lock()
		browser := s.host.browser
		s.host.mu.Unlock()
		if browser == nil {
			http.NotFound(w, r)
			return
		}
		browser.serve(w, r)
		return
	}
	if r.URL.Path == "/api/host/mcp" {
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		s.host.handler.ServeHTTP(w, r)
		return
	}
	var value any
	var e error
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/host/windows":
		value, e = s.hostWindows(r.Context())
	case r.Method == "POST" && r.URL.Path == "/api/host/register":
		var q HostRegistration
		if e = decode(r, &q); e == nil {
			value, e = s.hostRegisterContext(r.Context(), q)
		}
	case r.Method == "POST" && r.URL.Path == "/api/host/poll":
		var q HostIdentity
		if e = decode(r, &q); e == nil {
			value, e = s.hostPoll(r.Context(), q)
		}
	case r.Method == "POST" && r.URL.Path == "/api/host/result":
		var q HostResult
		if e = decode(r, &q); e == nil {
			e = s.hostResultContext(r.Context(), q)
		}
	case r.Method == "POST" && r.URL.Path == "/api/host/pairing/challenge":
		var q HostPairChallenge
		if e = decode(r, &q); e == nil {
			value, e = s.hostPairChallenge(r.Context(), q)
		}
	case r.Method == "POST" && r.URL.Path == "/api/host/pairing/attest":
		var q HostPairAttest
		if e = decode(r, &q); e == nil {
			value, e = s.hostPairAttest(r.Context(), q)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
