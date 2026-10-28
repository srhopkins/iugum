package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoordinationOptionalAndExactBackgroundTargets(t *testing.T) {
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}, Coordination: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	ref := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd}
	worker := handoffCreate(t, s, ref)
	q := CoordinationRequest{Owner: owner.ID, TaskID: testID, Conversation: worker.ID, Revision: worker.Revision, Session: ref, Text: "work separately", CommandID: otherID}
	if _, err := s.coordinate(context.Background(), "start", q); err == nil {
		t.Fatal("direct chat silently delegated")
	}
	if w := callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := s.coordinate(context.Background(), "start", q); err != nil {
		t.Fatal(err)
	}
	<-p.sessions[otherID].started
	if _, err := s.coordinate(context.Background(), "start", q); err == nil {
		t.Fatal("duplicate start accepted")
	}
	q.Owner = worker.ID
	if _, err := s.coordinate(context.Background(), "status", q); err == nil {
		t.Fatal("wrong owner read task")
	}
	q.Owner = owner.ID
	status, err := s.coordinate(context.Background(), "status", q)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(status)
	if !strings.Contains(string(b), "running") {
		t.Fatal("progress missing")
	}
	if _, err := s.coordinate(context.Background(), "stop", q); err != nil {
		t.Fatal(err)
	}
	close(p.sessions[otherID].finish)
	handoffAwait(t, s, worker.ID, "complete")
	status, err = s.coordinate(context.Background(), "status", q)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(status)
	if !strings.Contains(string(b), "provider answer") {
		t.Fatal("durable worker result missing")
	}
}

func TestCoordinationExplicitNewSessionReservedBeforeDispatch(t *testing.T) {
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}, Coordination: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	if w := callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	q := CoordinationRequest{Owner: owner.ID, TaskID: testID, Create: &contract.AgentSessionCreate{Provider: "Codex", Cwd: cwd}, CommandID: otherID, Text: "new worker"}
	value, err := s.coordinate(context.Background(), "start", q)
	if err != nil {
		t.Fatal(err)
	}
	task := value.(CoordinationTask)
	if task.Target.ID != otherID || task.Conversation == "" {
		t.Fatal("new worker identity missing")
	}
	awaitStarted(t, p.clients[otherID])
	original := p.clients[otherID]
	if _, err := s.coordinate(context.Background(), "start", q); err == nil {
		t.Fatal("creation retried")
	}
	if p.clients[otherID] != original {
		t.Fatal("duplicate replaced native session")
	}
	close(original.finish)
	handoffAwait(t, s, task.Conversation, "complete")
}

func TestCoordinationRestartAndPolicyRevocation(t *testing.T) {
	cfg := Config{DataDir: t.TempDir(), Coordination: true, Check: func(context.Context, string, string) error { return nil }}
	s, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ownerRef := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	// A crash reservation may exist before the native create returns its identity.
	s.coord.state.Enabled["owner-fixture"] = true
	s.coord.state.Tasks[testID] = CoordinationTask{ID: testID, Owner: "owner-fixture", State: "starting", Target: ownerRef}
	if err := s.saveCoordination(s.coord.state); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.coord.state.Tasks[testID].State != "uncertain" {
		t.Fatal("restart implied completion or replay")
	}
	s.cfg.Check = func(context.Context, string, string) error { return errors.New("revoked") }
	if _, err := s.coordinate(context.Background(), "status", CoordinationRequest{Owner: "owner-fixture", TaskID: testID}); err == nil {
		t.Fatal("revoked policy ignored")
	}
}
func TestCoordinationFollowupAndApprovalReporting(t *testing.T) {
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}, Coordination: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	ref := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd}
	worker := handoffCreate(t, s, ref)
	callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true})
	q := CoordinationRequest{Owner: owner.ID, TaskID: testID, Conversation: worker.ID, Revision: worker.Revision, Session: ref, Text: "work", CommandID: otherID}
	if _, err := s.coordinate(context.Background(), "start", q); err != nil {
		t.Fatal(err)
	}
	<-p.sessions[otherID].started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resolved := make(chan error, 1)
	go func() {
		_, err := p.sessions[otherID].observer.Request(ctx, contract.AgentSessionRequest{Method: "item/commandExecution/requestApproval"})
		resolved <- err
	}()
	deadline := time.Now().Add(time.Second)
	seen := false
	for time.Now().Before(deadline) {
		v, err := s.coordinate(context.Background(), "approvals", q)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), "requestApproval") {
			seen = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !seen {
		t.Fatal("approval not reported")
	}
	select {
	case <-resolved:
		t.Fatal("reporting inferred an approval")
	default:
	}
	cancel()
	close(p.sessions[otherID].finish)
	handoffAwait(t, s, worker.ID, "complete")
	q.CommandID = "33333333-3333-4333-8333-333333333333"
	q.Text = "follow up"
	q.Session.ID = testID
	if _, err := s.coordinate(context.Background(), "follow_up", q); err == nil {
		t.Fatal("follow-up redirected")
	}
	q.Session = ref
	if _, err := s.coordinate(context.Background(), "follow_up", q); err != nil {
		t.Fatal(err)
	}
	<-p.sessions[otherID].started
	handoffAwait(t, s, worker.ID, "complete")
	if _, err := s.coordinate(context.Background(), "follow_up", q); err == nil {
		t.Fatal("follow-up replayed")
	}
}

type coordinationAuth struct{ token string }

func (t coordinationAuth) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(r)
}
func TestCoordinationMCPOptionalTransportAndPolicy(t *testing.T) {
	disabled, _ := testServer(t)
	if w := callService(disabled, "POST", "/api/coordination/mcp", map[string]any{}); w.Code != 404 {
		t.Fatal("direct service exposed coordination")
	}
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{Providers: []contract.AgentSessionProvider{p}, DataDir: t.TempDir(), Coordination: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/api/coordination/mcp", HTTPClient: &http.Client{Transport: coordinationAuth{s.token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil || len(listed.Tools) != 5 {
		t.Fatalf("tools missing: %v %v", listed, err)
	}
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	ref := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd}
	worker := handoffCreate(t, s, ref)
	callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true})
	args := map[string]any{"owner": owner.ID, "taskId": testID, "conversation": worker.ID, "revision": worker.Revision, "session": ref, "text": "MCP fixture work", "commandId": otherID}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "coordination_start", Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("MCP start failed: %v %v", result, err)
	}
	select {
	case text := <-p.sessions[otherID].started:
		if !strings.Contains(text, "MCP fixture work") {
			t.Fatal("MCP arguments lost")
		}
	case <-time.After(time.Second):
		t.Fatal("MCP did not dispatch worker")
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "coordination_start", Arguments: args})
	if err == nil && !result.IsError {
		t.Fatal("duplicate MCP start accepted")
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "coordination_status", Arguments: map[string]any{"owner": owner.ID, "taskId": testID}})
	if err != nil || result.IsError {
		t.Fatalf("MCP status failed: %v %v", result, err)
	}
	close(p.sessions[otherID].finish)
	handoffAwait(t, s, worker.ID, "complete")
	s.cfg.Check = func(context.Context, string, string) error { return errors.New("revoked") }
	if _, err := session.ListTools(context.Background(), &mcp.ListToolsParams{}); err == nil {
		t.Fatal("MCP route ignored revoked policy")
	}
}

func TestCoordinationDisabledDoesNotReadOptionalStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coordination.json"), []byte("broken optional store"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), Config{DataDir: dir})
	if err != nil {
		t.Fatalf("disabled coordination broke direct chat: %v", err)
	}
	s.Close()
}

func TestCoordinationStopDoesNotTargetLaterTurnAndFollowupBlocksUncertainty(t *testing.T) {
	p := &handoffProvider{name: "Codex", sessions: map[string]*handoffSession{}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}, Coordination: true, Check: func(context.Context, string, string) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	ref := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: cwd}
	worker := handoffCreate(t, s, ref)
	callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true})
	q := CoordinationRequest{Owner: owner.ID, TaskID: testID, Conversation: worker.ID, Revision: worker.Revision, Session: ref, Text: "work", CommandID: otherID}
	if _, err := s.coordinate(context.Background(), "start", q); err != nil {
		t.Fatal(err)
	}
	<-p.sessions[otherID].started
	close(p.sessions[otherID].finish)
	handoffAwait(t, s, worker.ID, "complete")
	if _, err := s.coordinate(context.Background(), "stop", q); err == nil {
		t.Fatal("completed task can interrupt a later unrelated turn")
	}
	// Model a restart/connection failure: coordination reservation remains active,
	// while the authoritative canonical command outcome has become uncertain.
	s.convMu.Lock()
	c := copyConversation(s.conversations[worker.ID])
	c.Commands[0].Status = "uncertain"
	if err := s.putConversation(c); err != nil {
		t.Fatal(err)
	}
	s.convMu.Unlock()
	q.CommandID = "33333333-3333-4333-8333-333333333333"
	q.Text = "new direction"
	if _, err := s.coordinate(context.Background(), "follow_up", q); err == nil {
		t.Fatal("follow-up ignored canonical uncertainty")
	}
}

func TestCoordinationCreationPersistenceFailureRetainsKnownWorker(t *testing.T) {
	dir := t.TempDir()
	armed := false
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	s, err := New(context.Background(), Config{DataDir: dir, Coordination: true, Providers: []contract.AgentSessionProvider{p}, Check: func(_ context.Context, resource, action string) error {
		if armed && resource == "agent/sessions/api/conversations" && action == "write" {
			path := filepath.Join(dir, "coordination.json")
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Mkdir(path, 0700)
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cwd := t.TempDir()
	owner := handoffCreate(t, s, contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: cwd})
	callService(s, "POST", "/api/conversations/"+owner.ID+"/coordination", map[string]bool{"enabled": true})
	armed = true
	q := CoordinationRequest{Owner: owner.ID, TaskID: testID, Create: &contract.AgentSessionCreate{Provider: "Codex", Cwd: cwd}, CommandID: otherID, Text: "new worker"}
	if _, err := s.coordinate(context.Background(), "start", q); err == nil {
		t.Fatal("failed write hidden")
	}
	task := s.coord.state.Tasks[testID]
	if task.State != "uncertain" || task.Conversation == "" || task.Target.ID != otherID {
		t.Fatalf("known native/canonical worker lost: %+v", task)
	}
	select {
	case <-p.clients[otherID].started:
		t.Fatal("prompt sent after failed identity persistence")
	default:
	}
}
