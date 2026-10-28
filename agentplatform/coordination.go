package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CoordinationRequest grants no provider permission. Create is an explicit new
// session request; an omitted Create targets an exactly prepared worker conversation.
type CoordinationRequest struct {
	Create       *contract.AgentSessionCreate `json:"create,omitempty"`
	Owner        string                       `json:"owner"`
	TaskID       string                       `json:"taskId"`
	Conversation string                       `json:"conversation,omitempty"`
	Revision     uint64                       `json:"revision,omitempty"`
	Session      contract.AgentSessionRef     `json:"session,omitempty"`
	Text         string                       `json:"text,omitempty"`
	CommandID    string                       `json:"commandId,omitempty"`
}
type CoordinationTask struct {
	ID           string                   `json:"id"`
	Owner        string                   `json:"owner"`
	Conversation string                   `json:"conversation"`
	Target       contract.AgentSessionRef `json:"target"`
	Commands     []string                 `json:"commands"`
	State        string                   `json:"state"`
	Error        string                   `json:"error,omitempty"`
}
type coordinationState struct {
	Enabled map[string]bool             `json:"enabled"`
	Tasks   map[string]CoordinationTask `json:"tasks"`
}
type coordinationModule struct {
	mu      sync.Mutex
	state   coordinationState
	handler http.Handler
}

func (s *Server) initCoordination() error {
	s.coord = &coordinationModule{state: coordinationState{Enabled: map[string]bool{}, Tasks: map[string]CoordinationTask{}}}
	if !s.cfg.Coordination {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "coordination.json"))
	if err == nil {
		if err = json.Unmarshal(b, &s.coord.state); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if s.coord.state.Enabled == nil || s.coord.state.Tasks == nil {
		return errors.New("invalid coordination store")
	}
	for id, task := range s.coord.state.Tasks {
		if task.State == "starting" {
			task.State = "uncertain"
			task.Error = "service restarted during dispatch; inspect worker history; not replayed"
			s.coord.state.Tasks[id] = task
		}
	}
	if !s.cfg.Coordination {
		return nil
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "iugum-coordination", Version: "1"}, nil)
	for _, op := range []string{"start", "status", "follow_up", "stop", "approvals"} {
		operation := op
		mcp.AddTool(server, &mcp.Tool{Name: "coordination_" + operation, Description: "Optional explicit coordination: " + operation + " an exact worker task. Start accepts explicit create args OR a prepared canonical conversation. Requires owner conversation coordination enabled by the user. Never infers approval or changes models. Approvals only reports pending requests; the human replies on the original session."}, func(ctx context.Context, _ *mcp.CallToolRequest, q CoordinationRequest) (*mcp.CallToolResult, any, error) {
			result, err := s.coordinate(ctx, operation, q)
			if err != nil {
				return nil, nil, err
			}
			b, err := json.Marshal(result)
			if err != nil {
				return nil, nil, err
			}
			text := string(b)
			if len(text) > 32000 {
				text = text[:32000] + "\n[Result truncated; inspect worker conversation through the normal API.]"
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
		})
	}
	s.coord.handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return nil
}
func (s *Server) saveCoordination(state coordinationState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err = atomicFile(filepath.Join(s.cfg.DataDir, "coordination.json"), b); err != nil {
		return err
	}
	s.coord.state = state
	return nil
}
func (s *Server) copyCoordination() coordinationState {
	c := coordinationState{Enabled: map[string]bool{}, Tasks: map[string]CoordinationTask{}}
	for id, on := range s.coord.state.Enabled {
		c.Enabled[id] = on
	}
	for id, t := range s.coord.state.Tasks {
		t.Commands = append([]string{}, t.Commands...)
		c.Tasks[id] = t
	}
	return c
}
func (s *Server) coordAllowed(ctx context.Context, owner, task, action string) error {
	if !s.cfg.Coordination || s.cfg.Check == nil {
		return errors.New("optional coordination is not configured with a policy")
	}
	if err := s.cfg.Check(ctx, "agent/coordination/"+owner+"/"+task, action); err != nil {
		return errors.New("coordination policy denied")
	}
	if err := s.conversationCheck(ctx, owner, "coordinate"); err != nil {
		return errors.New("owner coordination policy denied")
	}
	s.convMu.Lock()
	_, exists := s.conversations[owner]
	s.convMu.Unlock()
	if !exists {
		return errors.New("unknown owner conversation")
	}
	return nil
}
func (s *Server) coordinationEnable(r *http.Request, id string) (any, error) {
	if r.Method != "POST" {
		return nil, errors.New("coordination mode requires explicit POST")
	}
	var q struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &q); err != nil {
		return nil, err
	}
	if err := s.coordAllowed(r.Context(), id, "mode", "enable"); err != nil {
		return nil, err
	}
	s.coord.mu.Lock()
	defer s.coord.mu.Unlock()
	state := s.copyCoordination()
	state.Enabled[id] = q.Enabled
	if err := s.saveCoordination(state); err != nil {
		return nil, err
	}
	return map[string]bool{"enabled": q.Enabled}, nil
}

// Invoke the same API policy and route implementations used by the editor.
func (s *Server) coordCall(ctx context.Context, method, path string, body any) (any, error) {
	action := "read"
	if method != "GET" {
		action = "write"
	}
	if s.cfg.Check == nil {
		return nil, errors.New("coordination policy required")
	}
	if err := s.cfg.Check(ctx, "agent/sessions"+strings.Split(path, "?")[0], action); err != nil {
		return nil, errors.New("session route policy denied")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1"+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return s.route(nil, r)
}
func (s *Server) coordinate(ctx context.Context, op string, q CoordinationRequest) (any, error) {
	if err := s.coordAllowed(ctx, q.Owner, q.TaskID, op); err != nil {
		return nil, err
	}
	if !uuid.MatchString(q.TaskID) {
		return nil, errors.New("explicit task UUID required")
	}
	s.coord.mu.Lock()
	defer s.coord.mu.Unlock()
	if !s.coord.state.Enabled[q.Owner] {
		return nil, errors.New("owner is in direct mode; explicitly enable coordination first")
	}
	state := s.copyCoordination()
	task, exists := state.Tasks[q.TaskID]
	if op == "start" {
		if exists {
			return nil, errors.New("task already reserved; inspect status; not replayed")
		}
		if !uuid.MatchString(q.CommandID) || strings.TrimSpace(q.Text) == "" {
			return nil, errors.New("command UUID and prompt required")
		}
		var err error
		if q.Create != nil {
			if q.Conversation != "" || q.Session.ID != "" {
				return nil, errors.New("choose explicit new session creation or exact existing conversation")
			}
			task = CoordinationTask{ID: q.TaskID, Owner: q.Owner, Commands: []string{q.CommandID}, State: "starting"}
			state.Tasks[task.ID] = task
			if err = s.saveCoordination(state); err != nil {
				return nil, err
			}
			// Reserve before provider-native creation, never replay an uncertain create.
			var created any
			created, err = s.coordCall(ctx, "POST", "/api/sessions", q.Create)
			if err == nil {
				task.Target = created.(map[string]any)["session"].(contract.AgentSessionRef)
				state = s.copyCoordination()
				state.Tasks[task.ID] = task
				err = s.saveCoordination(state)
			}
			if err == nil {
				var value any
				value, err = s.coordCall(ctx, "POST", "/api/conversations", map[string]any{"session": task.Target, "importHistory": false, "title": q.Create.Title})
				if err == nil {
					worker := value.(contract.AgentConversation)
					task.Conversation = worker.ID
					q.Revision = worker.Revision
				}
			}
			if err != nil {
				task.State = "uncertain"
				task.Error = "worker creation not confirmed; inspect native catalog and task; no automatic replay: " + err.Error()
				state = s.copyCoordination()
				state.Tasks[task.ID] = task
				if saveErr := s.saveCoordination(state); saveErr != nil {
					s.coord.state.Tasks[task.ID] = task
				}
				return task, err
			}
		} else {
			if q.Conversation == q.Owner || q.Conversation == "" {
				return nil, errors.New("explicit separate worker conversation required")
			}
			for _, t := range state.Tasks {
				if t.Conversation == q.Conversation {
					return nil, errors.New("worker conversation already belongs to a coordination task")
				}
			}
			var value any
			value, err = s.coordCall(ctx, "GET", "/api/conversations/"+q.Conversation, nil)
			if err != nil {
				return nil, err
			}
			worker := value.(contract.AgentConversation)
			if worker.Revision != q.Revision || !sameSession(worker.Selected, q.Session) {
				return nil, errors.New("worker target changed; refresh before starting")
			}
			task = CoordinationTask{ID: q.TaskID, Owner: q.Owner, Conversation: q.Conversation, Target: q.Session, Commands: []string{q.CommandID}, State: "starting"}
		}
		state = s.copyCoordination()
		state.Tasks[task.ID] = task
		if err = s.saveCoordination(state); err != nil {
			task.State = "uncertain"
			task.Error = "coordination persistence failed before prompt dispatch; inspect known worker links"
			s.coord.state.Tasks[task.ID] = task
			return task, err
		}
		_, err = s.coordCall(ctx, "POST", "/api/conversations/"+task.Conversation+"/chat", map[string]any{"revision": q.Revision, "session": task.Target, "text": q.Text, "commandId": q.CommandID})
		task.State = "active"
		if err != nil {
			task.State = "uncertain"
			task.Error = "dispatch not confirmed; inspect worker history before another attempt: " + err.Error()
		}
		state = s.copyCoordination()
		state.Tasks[task.ID] = task
		if saveErr := s.saveCoordination(state); saveErr != nil {
			task.State = "uncertain"
			task.Error = "coordination persistence failed; inspect worker history"
			s.coord.state.Tasks[task.ID] = task
			return task, errors.New(task.Error)
		}
		return task, err
	}
	if !exists || task.Owner != q.Owner {
		return nil, errors.New("unknown task for this owner")
	}
	if task.Conversation == "" {
		if op == "status" {
			return map[string]any{"task": task}, nil
		}
		return nil, errors.New("worker creation uncertain; inspect original native catalog before another action")
	}
	// Every action additionally rechecks target conversation access.
	value, err := s.coordCall(ctx, "GET", "/api/conversations/"+task.Conversation, nil)
	if err != nil {
		return nil, err
	}
	worker := value.(contract.AgentConversation)
	switch op {
	case "status":
		// Canonical command state is authoritative for completion; bridge progress is separate.
		if len(task.Commands) > 0 {
			last := task.Commands[len(task.Commands)-1]
			for _, command := range worker.Commands {
				if command.ID == last {
					task.State = command.Status
					if command.Error != "" {
						task.Error = command.Error
					}
				}
			}
		}
		if _, err := s.coordCall(ctx, "POST", "/api/select", task.Target); err != nil {
			return map[string]any{"task": task, "conversation": worker, "connectionError": err.Error()}, nil
		}
		prefix := "/api/agents/" + providerRoute(task.Target.Provider) + "/"
		query := "?conversation=" + task.Target.ID
		status, err := s.coordCall(ctx, "GET", prefix+"status"+query, nil)
		if err != nil {
			return nil, err
		}
		progress, err := s.coordCall(ctx, "GET", prefix+"events"+query, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"task": task, "conversation": worker, "status": status, "progress": progress}, nil
	case "follow_up":
		complete := false
		if len(task.Commands) > 0 {
			last := task.Commands[len(task.Commands)-1]
			for _, cmd := range worker.Commands {
				if cmd.ID == last {
					complete = cmd.Status == "complete"
				}
			}
		}
		if !complete {
			return nil, errors.New("worker command is running or uncertain; inspect status before further direction")
		}
		if task.State == "starting" || task.State == "uncertain" {
			return nil, errors.New("task dispatch uncertain; inspect history before further direction")
		}
		if !sameSession(q.Session, task.Target) || q.Conversation != task.Conversation {
			return nil, errors.New("follow-up must retain exact original worker target")
		}
		for _, id := range task.Commands {
			if id == q.CommandID {
				return nil, errors.New("command already reserved; not replayed")
			}
		}
		if !uuid.MatchString(q.CommandID) || strings.TrimSpace(q.Text) == "" {
			return nil, errors.New("command UUID and prompt required")
		}
		task.Commands = append(task.Commands, q.CommandID)
		task.State = "starting"
		state.Tasks[task.ID] = task
		if err := s.saveCoordination(state); err != nil {
			return nil, err
		}
		_, err = s.coordCall(ctx, "POST", "/api/conversations/"+task.Conversation+"/chat", map[string]any{"revision": q.Revision, "session": task.Target, "text": q.Text, "commandId": q.CommandID})
		task.State = "active"
		if err != nil {
			task.State = "uncertain"
			task.Error = "follow-up not confirmed; inspect history: " + err.Error()
		}
		state = s.copyCoordination()
		state.Tasks[task.ID] = task
		if saveErr := s.saveCoordination(state); saveErr != nil {
			task.State = "uncertain"
			task.Error = "coordination persistence failed; inspect worker history"
			s.coord.state.Tasks[task.ID] = task
			return task, errors.New(task.Error)
		}
		return task, err
	case "stop":
		return s.coordinationStop(ctx, task)
	case "approvals":
		route := providerRoute(task.Target.Provider)
		if route == "" {
			return nil, errors.New("worker provider unavailable")
		}
		// Reopen exactly the reserved execution session after service restart.
		if _, err := s.coordCall(ctx, "POST", "/api/select", task.Target); err != nil {
			return nil, err
		}
		return s.coordCall(ctx, "GET", "/api/agents/"+route+"/approvals?conversation="+task.Target.ID, nil)
	default:
		return nil, errors.New("unknown coordination operation")
	}
}

// Serialize the ownership check with new canonical sends. An old task cannot
// cancel a later direct turn using the same provider-native session.
func (s *Server) coordinationStop(ctx context.Context, task CoordinationTask) (any, error) {
	s.convMu.Lock()
	defer s.convMu.Unlock()
	worker, ok := s.conversations[task.Conversation]
	if !ok || len(task.Commands) == 0 {
		return nil, errors.New("worker task unavailable")
	}
	var latest contract.AgentConversationCommand
	for _, cmd := range worker.Commands {
		if sameSession(cmd.Session, task.Target) {
			latest = cmd
		}
	}
	if latest.ID != task.Commands[len(task.Commands)-1] || latest.Status != "running" {
		return nil, errors.New("task has no active owned turn; stop cannot target later work")
	}
	if err := s.cfg.Check(ctx, "agent/sessions/"+task.Target.Provider+"/"+task.Target.ID, "interrupt"); err != nil {
		return nil, errors.New("worker interruption policy denied")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.coordCall(ctx, "POST", "/api/agents/"+providerRoute(task.Target.Provider)+"/interrupt?conversation="+task.Target.ID, map[string]any{})
}
