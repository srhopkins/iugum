// Package agentcontrol owns persistent, bidirectional coding-agent processes.
package agentcontrol

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/contract"
)

var ErrUnmanaged = errors.New("session is not managed by this process; reconnect its saved session or start a managed session")
var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

type StartRequest struct {
	Actor           string        `json:"actor"`
	ID              string        `json:"id"`
	ResumeSessionID string        `json:"resume_session_id,omitempty"`
	Project         string        `json:"project"`
	Account         string        `json:"account"`
	ConfigDir       string        `json:"config_dir"`
	Model           string        `json:"model,omitempty"`
	AllowedTools    []string      `json:"allowed_tools,omitempty"`
	MaxDuration     time.Duration `json:"max_duration,omitempty"`
}
type Pending struct {
	ID       string    `json:"id"`
	Actor    string    `json:"actor"`
	Prompt   string    `json:"prompt"`
	QueuedAt time.Time `json:"queued_at"`
}
type State struct {
	Usage      SessionUsage `json:"usage"`
	ID         string       `json:"id"`
	SessionID  string       `json:"session_id,omitempty"`
	Status     string       `json:"status"`
	Request    StartRequest `json:"request"`
	Pending    []Pending    `json:"pending,omitempty"`
	UpdatedAt  time.Time    `json:"updated_at"`
	LastResult string       `json:"last_result,omitempty"`
	Error      string       `json:"error,omitempty"`
	PID        int          `json:"pid,omitempty"`
}
type Event struct {
	Time      time.Time       `json:"time"`
	Direction string          `json:"direction"`
	Data      json.RawMessage `json:"data"`
}
type process struct {
	state   State
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	cancel  context.CancelFunc
	done    chan struct{}
	writeMu sync.Mutex
	acks    map[string]chan error
}
type Manager struct {
	// VerifyAccount is an injection seam for tests or stricter host identity checks. Nil uses Claude auth status.
	VerifyAccount func(context.Context, string, StartRequest) error
	journalMu     sync.Mutex
	root          string
	gate          contract.Policy
	Executable    string
	Now           func() time.Time
	mu            sync.Mutex
	processes     map[string]*process
}

func New(root string, gate contract.Policy) (*Manager, error) {
	if gate == nil {
		return nil, errors.New("agentcontrol: policy required")
	}
	if root == "" {
		return nil, errors.New("agentcontrol: state directory required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Manager{root: root, gate: gate, Now: time.Now, processes: map[string]*process{}}, nil
}
func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func (m *Manager) authorize(ctx context.Context, actor, id, action string) error {
	if actor == "" || !validID.MatchString(id) {
		return errors.New("agentcontrol: valid actor and session ID required")
	}
	return m.gate.Enforce(ctx, contract.Request{Sub: actor, Obj: "agent/session/" + id, Act: action})
}
func (m *Manager) path(id, suffix string) string { return filepath.Join(m.root, id+suffix) }
func (m *Manager) save(p *process) error {
	p.state.UpdatedAt = m.Now().UTC()
	b, err := json.MarshalIndent(p.state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(m.root, ".control-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), m.path(p.state.ID, ".json"))
}
func (m *Manager) event(p *process, direction string, data []byte) error {
	m.journalMu.Lock()
	defer m.journalMu.Unlock()
	b, err := json.Marshal(Event{Time: m.Now().UTC(), Direction: direction, Data: json.RawMessage(data)})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(m.path(p.state.ID, ".events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
func clone(s State) State {
	b, _ := json.Marshal(s)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}
func (m *Manager) Start(ctx context.Context, r StartRequest) (State, error) {
	if r.ID == "" {
		r.ID = randomID()
	}
	if err := m.authorize(ctx, r.Actor, r.ID, "start"); err != nil {
		return State{}, err
	}
	if r.ResumeSessionID != "" && !validID.MatchString(r.ResumeSessionID) {
		return State{}, errors.New("agentcontrol: invalid resume session ID")
	}
	if r.Account == "" || r.ConfigDir == "" || r.Project == "" {
		return State{}, errors.New("agentcontrol: project, explicit account and config directory required")
	}
	for _, dir := range []string{r.Project, r.ConfigDir} {
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			return State{}, fmt.Errorf("agentcontrol: directory unavailable: %s", dir)
		}
	}
	if r.MaxDuration <= 0 {
		r.MaxDuration = 8 * time.Hour
	}
	if r.MaxDuration > 24*time.Hour {
		return State{}, errors.New("agentcontrol: duration cannot exceed 24 hours")
	}
	m.mu.Lock()
	if _, exists := m.processes[r.ID]; exists {
		m.mu.Unlock()
		return State{}, errors.New("agentcontrol: managed ID already active")
	}
	m.mu.Unlock()
	binary := m.Executable
	if binary == "" {
		binary = "claude"
	}
	verify := m.VerifyAccount
	if verify == nil {
		verify = verifyClaudeAccount
	}
	if err := verify(ctx, binary, r); err != nil {
		return State{}, err
	}
	args := []string{"--print", "--verbose", "--input-format", "stream-json", "--output-format", "stream-json", "--replay-user-messages", "--permission-mode", "manual", "--permission-prompts", "none"}
	if r.ResumeSessionID != "" {
		args = append(args, "--resume", r.ResumeSessionID)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if len(r.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(r.AllowedTools, ","))
	}
	runCtx, cancel := context.WithTimeout(ctx, r.MaxDuration)
	cmd := exec.CommandContext(runCtx, binary, args...)
	cmd.Dir = r.Project
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 3 * time.Second
	cmd.Env = claudeEnvironment(r.ConfigDir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return State{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return State{}, err
	}
	p := &process{state: State{ID: r.ID, SessionID: r.ResumeSessionID, Status: "starting", Request: r}, cmd: cmd, stdin: stdin, cancel: cancel, done: make(chan struct{}), acks: map[string]chan error{}}
	m.mu.Lock()
	if _, exists := m.processes[r.ID]; exists {
		m.mu.Unlock()
		cancel()
		return State{}, errors.New("agentcontrol: managed ID already active")
	}
	m.processes[r.ID] = p
	if err = m.save(p); err != nil {
		delete(m.processes, r.ID)
		m.mu.Unlock()
		cancel()
		return State{}, err
	}
	if err = cmd.Start(); err != nil {
		p.state.Status = "failed"
		p.state.Error = err.Error()
		_ = m.save(p)
		delete(m.processes, r.ID)
		m.mu.Unlock()
		cancel()
		return State{}, err
	}
	p.state.PID = cmd.Process.Pid
	_ = m.save(p)
	m.mu.Unlock()
	go m.read(p, stdout)
	initCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	if err = m.control(initCtx, p, "initialize"); err != nil {
		cancel()
		return State{}, err
	}
	m.mu.Lock()
	if p.state.Status == "starting" {
		p.state.Status = "idle"
	}
	err = m.save(p)
	state := clone(p.state)
	m.mu.Unlock()
	return state, err
}
func (m *Manager) write(p *process, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if err = m.event(p, "input", b); err != nil {
		return err
	}
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}
func (m *Manager) control(ctx context.Context, p *process, subtype string) error {
	id := randomID()
	ack := make(chan error, 1)
	m.mu.Lock()
	p.acks[id] = ack
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(p.acks, id); m.mu.Unlock() }()
	if err := m.write(p, map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": subtype}}); err != nil {
		return err
	}
	select {
	case err := <-ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("agentcontrol: process exited before control acknowledgement")
	}
}
func (m *Manager) read(p *process, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	var readErr error
	for scanner.Scan() {
		b := append([]byte{}, scanner.Bytes()...)
		var ev struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Result    string `json:"result"`
			IsError   bool   `json:"is_error"`
			Response  struct {
				RequestID string `json:"request_id"`
				Subtype   string `json:"subtype"`
				Error     string `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal(b, &ev); err != nil {
			readErr = fmt.Errorf("invalid Claude event: %w", err)
			p.cancel()
			break
		}
		m.mu.Lock()
		if err := m.event(p, "output", b); err != nil {
			readErr = err
			p.cancel()
			m.mu.Unlock()
			break
		}
		if ev.SessionID != "" {
			p.state.SessionID = ev.SessionID
		}
		if ev.Type == "control_response" {
			if ch := p.acks[ev.Response.RequestID]; ch != nil {
				var err error
				if ev.Response.Subtype == "error" {
					err = errors.New(ev.Response.Error)
				}
				select {
				case ch <- err:
				default:
				}
			}
		}
		if ev.Type == "result" {
			p.state.Status = "idle"
			p.state.LastResult = ev.Result
			p.state.Usage = latestClaudeUsage(b, m.Now())
			if ev.IsError {
				p.state.Error = ev.Result
			}
		}
		if err := m.save(p); err != nil {
			readErr = err
			p.cancel()
		}
		drain := ev.Type == "result"
		m.mu.Unlock()
		if drain {
			m.deliver(p)
		}
	}
	if err := scanner.Err(); err != nil {
		readErr = err
	}
	waitErr := p.cmd.Wait()
	p.cancel()
	m.mu.Lock()
	p.state.PID = 0
	p.state.Status = "stopped"
	if readErr != nil {
		p.state.Status = "failed"
		p.state.Error = readErr.Error()
	} else if waitErr != nil {
		p.state.Error = waitErr.Error()
	}
	_ = m.save(p)
	delete(m.processes, p.state.ID)
	close(p.done)
	m.mu.Unlock()
}
func (m *Manager) deliver(p *process) {
	m.mu.Lock()
	if p.state.Status != "idle" || len(p.state.Pending) == 0 {
		m.mu.Unlock()
		return
	}
	pending := p.state.Pending[0]
	if err := m.authorize(context.Background(), pending.Actor, p.state.ID, "send"); err != nil {
		p.state.Error = err.Error()
		p.state.Status = "blocked"
		_ = m.save(p)
		m.mu.Unlock()
		return
	}
	p.state.Pending = p.state.Pending[1:]
	p.state.Status = "running"
	if err := m.save(p); err != nil {
		p.state.Error = err.Error()
		p.cancel()
		m.mu.Unlock()
		return
	}
	sessionID := p.state.SessionID
	m.mu.Unlock()
	content := "Current timestamp: " + m.Now().UTC().Format(time.RFC3339Nano) + ". Use recorded dates for historical facts.\n\n" + pending.Prompt
	err := m.write(p, map[string]any{"type": "user", "session_id": sessionID, "parent_tool_use_id": nil, "uuid": pending.ID, "message": map[string]any{"role": "user", "content": content}})
	if err != nil {
		m.mu.Lock()
		p.state.Error = err.Error()
		p.state.Status = "failed"
		_ = m.save(p)
		m.mu.Unlock()
		p.cancel()
	}
}
func (m *Manager) Send(ctx context.Context, actor, id, prompt string) (State, error) {
	if err := m.authorize(ctx, actor, id, "send"); err != nil {
		return State{}, err
	}
	if strings.TrimSpace(prompt) == "" || len(prompt) > 1<<20 {
		return State{}, errors.New("agentcontrol: prompt must be nonempty and at most 1 MiB")
	}
	m.mu.Lock()
	p := m.processes[id]
	if p == nil {
		m.mu.Unlock()
		return State{}, ErrUnmanaged
	}
	if len(p.state.Pending) >= 100 {
		m.mu.Unlock()
		return State{}, errors.New("agentcontrol: queue full")
	}
	pending := Pending{ID: randomID(), Actor: actor, Prompt: prompt, QueuedAt: m.Now().UTC()}
	queued, _ := json.Marshal(map[string]any{"type": "queued", "message": pending})
	if err := m.event(p, "control", queued); err != nil {
		m.mu.Unlock()
		return State{}, err
	}
	p.state.Pending = append(p.state.Pending, pending)
	err := m.save(p)
	m.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	m.deliver(p)
	return m.Snapshot(ctx, actor, id)
}
func (m *Manager) Interrupt(ctx context.Context, actor, id string) error {
	if err := m.authorize(ctx, actor, id, "interrupt"); err != nil {
		return err
	}
	m.mu.Lock()
	p := m.processes[id]
	m.mu.Unlock()
	if p == nil {
		return ErrUnmanaged
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return m.control(bounded, p, "interrupt")
}
func (m *Manager) Stop(ctx context.Context, actor, id string) error {
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if err := m.authorize(ctx, actor, id, "stop"); err != nil {
		return err
	}
	m.mu.Lock()
	p := m.processes[id]
	m.mu.Unlock()
	if p == nil {
		return ErrUnmanaged
	}
	_ = p.stdin.Close()
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) Snapshot(ctx context.Context, actor, id string) (State, error) {
	if err := m.authorize(ctx, actor, id, "read"); err != nil {
		return State{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.processes[id]; p != nil {
		return clone(p.state), nil
	}
	b, err := os.ReadFile(m.path(id, ".json"))
	var state State
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(b, &state)
	if state.Status == "running" || state.Status == "idle" || state.Status == "starting" {
		state.Status = "disconnected"
		state.PID = 0
	}
	return state, err
}
func (m *Manager) List(ctx context.Context, actor string) ([]State, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	states := []State{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		s, err := m.Snapshot(ctx, actor, id)
		if err == nil {
			states = append(states, s)
		}
	}
	return states, nil
}
func (m *Manager) Events(ctx context.Context, actor, id string, limit int) ([]Event, error) {
	if err := m.authorize(ctx, actor, id, "read"); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	m.journalMu.Lock()
	defer m.journalMu.Unlock()
	f, err := os.Open(m.path(id, ".events.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	events := []Event{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 9<<20)
	for scanner.Scan() {
		var ev Event
		if err = json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
		if len(events) > limit {
			events = events[1:]
		}
	}
	return events, scanner.Err()
}
func (m *Manager) Reconnect(ctx context.Context, actor, id string) (State, error) {
	old, err := m.Snapshot(ctx, actor, id)
	if err != nil {
		return old, err
	}
	if old.SessionID == "" {
		return old, errors.New("agentcontrol: no saved Claude session ID")
	}
	r := old.Request
	r.Actor = actor
	r.ResumeSessionID = old.SessionID
	state, err := m.Start(ctx, r)
	if err != nil {
		return state, err
	} // Pending prompts are never replayed automatically after an uncertain disconnect.
	return state, nil
}
