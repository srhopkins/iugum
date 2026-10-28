package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCanonicalControlIncarnationRotatesOnRestart(t *testing.T) {
	s, _ := testServer(t)
	get := func(s *Server) string {
		t.Helper()
		w := callService(s, "GET", "/api/control-incarnation", nil)
		if w.Code != 200 {
			t.Fatalf("incarnation discovery: %s", w.Body.String())
		}
		var v map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if len(v["incarnation"]) < 32 {
			t.Fatal("missing incarnation")
		}
		return v["incarnation"]
	}
	before := get(s)
	s.Close()
	next, err := New(context.Background(), s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if before == get(next) {
		t.Fatal("incarnation reused across restart")
	}
}
func TestCanonicalControlPreparedModelIsBoundAndSingleUse(t *testing.T) {
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	w := callService(s, "POST", "/api/conversations", map[string]any{"session": ref})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	json.Unmarshal(w.Body.Bytes(), &c)
	w = callService(s, "GET", "/api/control-incarnation", nil)
	var identity map[string]string
	json.Unmarshal(w.Body.Bytes(), &identity)
	q := map[string]any{"phase": "prepare", "incarnation": identity["incarnation"], "window": "window-one", "actionId": "11111111-1111-4111-8111-111111111111", "revision": c.Revision, "session": ref, "kind": "model", "model": "test-model"}
	path := "/api/conversations/" + c.ID + "/control"
	w = callService(s, "POST", path, q)
	if w.Code != 200 {
		t.Fatalf("prepare: %s", w.Body.String())
	}
	var prepared map[string]any
	json.Unmarshal(w.Body.Bytes(), &prepared)
	prepared["phase"] = "commit"
	w = callService(s, "POST", path, prepared)
	if w.Code != 200 {
		t.Fatalf("commit: %s", w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["status"] != "completed" {
		t.Fatalf("not completed: %s", w.Body.String())
	}
	w = callService(s, "POST", path, prepared)
	if w.Code != 200 {
		t.Fatalf("duplicate status: %s", w.Body.String())
	}
	prepared["window"] = "other-window"
	if w = callService(s, "POST", path, prepared); w.Code == 200 {
		t.Fatal("changed window accepted")
	}
}

func conditionalFixture(t *testing.T) (*Server, contract.AgentConversation) {
	t.Helper()
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	w := callService(s, "POST", "/api/conversations", map[string]any{"session": contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	json.Unmarshal(w.Body.Bytes(), &c)
	return s, c
}
func prepareConditional(t *testing.T, s *Server, c contract.AgentConversation, kind string) *canonicalControl {
	t.Helper()
	q := canonicalControl{Phase: "prepare", Incarnation: s.controls.incarnation, Window: "test-window", ActionID: operationFixtureID(), Revision: c.Revision, Session: c.Selected, Kind: kind}
	if kind == "model" {
		q.Model = "test-model"
	}
	return &q
}
func frozenConditional(t *testing.T, s *Server, c contract.AgentConversation, q canonicalControl) canonicalControl {
	t.Helper()
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
	if w.Code != 200 {
		t.Fatalf("prepare: %s", w.Body.String())
	}
	var frozen canonicalControl
	if err := json.Unmarshal(w.Body.Bytes(), &frozen); err != nil {
		t.Fatal(err)
	}
	frozen.Phase = "commit"
	return frozen
}
func TestCanonicalControlStaleBindingsAndExpiry(t *testing.T) {
	for _, mutate := range []string{"revision", "native", "digest", "model", "expiry", "window"} {
		t.Run(mutate, func(t *testing.T) {
			s, c := conditionalFixture(t)
			q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
			switch mutate {
			case "revision":
				q.Revision++
			case "native":
				q.Session.ID = otherID
			case "digest":
				q.Digest = "bad"
			case "model":
				q.Model = "arbitrary"
			case "window":
				q.Window = "foreign"
			case "expiry":
				s.controls.mu.Lock()
				receipt := s.controls.receipts[q.ActionID]
				receipt.Proposal.ExpiresAt = time.Now().Add(-time.Second)
				q.ExpiresAt = receipt.Proposal.ExpiresAt
				q.Digest = controlDigest(c.ID, q)
				receipt.Digest = q.Digest
				s.controls.mu.Unlock()
			}
			if w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q); w.Code == 200 {
				t.Fatalf("stale %s accepted: %s", mutate, w.Body.String())
			}
		})
	}
}
func TestCanonicalControlRestartRejectsLateOldCommit(t *testing.T) {
	s, c := conditionalFixture(t)
	q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	path := "/api/conversations/" + c.ID + "/control"
	if w := callService(s, "POST", path, q); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	s.Close()
	next, err := New(context.Background(), s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if w := callService(next, "POST", path, q); w.Code == 200 {
		t.Fatal("late old incarnation replayed")
	}
	q.Phase = "prepare"
	q.Digest = ""
	q.ExpiresAt = time.Time{}
	q.ModelsDigest = ""
	if w := callService(next, "POST", path, q); w.Code == 200 {
		t.Fatal("old capture upgraded after restart")
	}
}
func TestCanonicalControlConcurrentDuplicateAndBusy(t *testing.T) {
	s, c := conditionalFixture(t)
	q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	m := s.sessions["Codex:"+testID]
	m.mu.Lock()
	m.busy = true
	m.mu.Unlock()
	path := "/api/conversations/" + c.ID + "/control"
	if w := callService(s, "POST", path, q); w.Code == 200 {
		t.Fatal("busy accepted")
	}
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
	w := callService(s, "POST", path, q)
	if !strings.Contains(w.Body.String(), "rejected") {
		t.Fatal("consumed rejected action retried")
	}
	q = frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := callService(s, "POST", path, q); w.Code != 200 {
				t.Errorf("duplicate %s", w.Body.String())
			}
		}()
	}
	wg.Wait()
}
func TestCanonicalControlChangedCurrentModelOrSelection(t *testing.T) {
	for _, change := range []string{"model", "selection"} {
		t.Run(change, func(t *testing.T) {
			s, c := conditionalFixture(t)
			q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
			if change == "model" {
				if w := callService(s, "POST", "/api/agents/codex-session/models?conversation="+testID, map[string]string{"model": "test-model"}); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			} else {
				if w := callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": c.Selected}); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
			w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
			if w.Code == 200 && !strings.Contains(w.Body.String(), "rejected") {
				t.Fatal("changed target accepted: " + w.Body.String())
			}
		})
	}
}
func TestCanonicalControlOriginalRequestOriginAndSingleAnswer(t *testing.T) {
	s, c := conditionalFixture(t)
	command := operationFixtureID()
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/chat", map[string]any{"revision": c.Revision, "session": c.Selected, "text": "fixture", "commandId": command})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	m := s.sessions["Codex:"+testID]
	client := m.client.(*controlsSession).AgentSession.(*testSession)
	<-client.started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	answers := make(chan json.RawMessage, 1)
	go func() {
		answer, _ := m.Request(ctx, contract.AgentSessionRequest{Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"availableDecisions":["accept","decline"]}`)})
		answers <- answer
	}()
	var a *approval
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		m.mu.Lock()
		for _, v := range m.requests {
			a = v
		}
		m.mu.Unlock()
		if a != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if a == nil || a.Origin == nil || a.Origin.Conversation != c.ID || a.Origin.CommandID != command {
		t.Fatal("canonical request origin not projected")
	}
	q := prepareConditional(t, s, c, "request")
	q.RequestID = a.ID
	q.RequestDigest = a.Digest
	q.Result = json.RawMessage(`{"decision":"decline"}`)
	frozen := frozenConditional(t, s, c, *q)
	path := "/api/conversations/" + c.ID + "/control"
	if w := callService(s, "POST", path, frozen); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	select {
	case got := <-answers:
		if string(got) != string(q.Result) {
			t.Fatal("wrong answer")
		}
	case <-time.After(time.Second):
		t.Fatal("answer not delivered")
	}
	if w := callService(s, "POST", "/api/agents/codex-session/approve?conversation="+testID, map[string]any{"id": a.ID, "digest": a.Digest, "result": map[string]string{"decision": "accept"}}); w.Code == 200 {
		t.Fatal("manual second answer won")
	}
	close(client.finish)
}
func TestCanonicalControlUnknownOriginAndUnsupportedDecisions(t *testing.T) {
	s, c := conditionalFixture(t)
	m := s.sessions["Codex:"+testID]
	a := &approval{AgentSessionRequest: contract.AgentSessionRequest{ID: "request", Method: "permission.asked", Params: json.RawMessage(`{}`)}, Digest: "known", context: context.Background(), answer: make(chan json.RawMessage, 1)}
	m.mu.Lock()
	m.requests[a.ID] = a
	m.mu.Unlock()
	q := prepareConditional(t, s, c, "request")
	q.RequestID = a.ID
	q.RequestDigest = a.Digest
	q.Result = json.RawMessage(`{"reply":"once"}`)
	if w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q); w.Code == 200 {
		t.Fatal("unknown origin inferred")
	}
	for _, tc := range []struct{ method, params, result string }{{"cursor/create_plan", `{}`, `{"outcome":{"outcome":"accepted"}}`}, {"can_use_tool", `{"input":{"command":"safe"}}`, `{"behavior":"allow","updatedInput":{"command":"different"}}`}, {"question.asked", `{"questions":[{"options":[{"label":"one"}],"multiple":true}]}`, `{"answers":[["one"]]}`}, {"question.asked", `{"questions":[{"options":[{"label":"one"}],"isSecret":true}]}`, `{"answers":[["one"]]}`}, {"question.asked", `{"questions":[{"options":[{"label":"one"}]}]}`, `{"answers":[["freeform"]]}`}} {
		if err := validateConditionalAnswer(contract.AgentSessionRequest{Method: tc.method, Params: json.RawMessage(tc.params)}, json.RawMessage(tc.result)); err == nil {
			t.Fatalf("unsafe decision accepted %s", tc.method)
		}
	}
}

func operationFixtureID() string {
	id := newControlID()
	return id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
}

type conditionalModel struct {
	contract.AgentSession
	mu         sync.Mutex
	current    string
	selectable bool
	calls      int
	started    chan struct{}
	release    chan struct{}
	failure    bool
	check      func()
}

func (m *conditionalModel) Models(context.Context) (contract.AgentSessionModels, error) {
	if m.check != nil {
		m.check()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return contract.AgentSessionModels{Options: []contract.AgentSessionModel{{ID: "test-model", Name: "Test"}}, Current: m.current, CanSelect: m.selectable}, nil
}
func (m *conditionalModel) SelectModel(ctx context.Context, id string) error {
	if m.check != nil {
		m.check()
	}
	m.mu.Lock()
	m.calls++
	started, release, failure := m.started, m.release, m.failure
	m.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if failure {
		return errors.New("private provider error must not leak")
	}
	m.mu.Lock()
	m.current = id
	m.mu.Unlock()
	return nil
}
func conditionalCounter(t *testing.T, s *Server) *conditionalModel {
	t.Helper()
	m := s.sessions["Codex:"+testID]
	client := &conditionalModel{AgentSession: m.client, selectable: true}
	m.client = client
	return client
}
func TestCanonicalControlReservationReleasesLocksBeforeProviderIO(t *testing.T) {
	s, c := conditionalFixture(t)
	client := conditionalCounter(t, s)
	m := s.sessions["Codex:"+testID]
	client.check = func() {
		for name, lock := range map[string]*sync.Mutex{"conversation": &s.convMu, "service": &s.mu, "session": &m.mu, "receipt": &s.controls.mu} {
			if !lock.TryLock() {
				t.Errorf("provider I/O under %s lock", name)
			} else {
				lock.Unlock()
			}
		}
	}
	q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	client.started = make(chan struct{})
	client.release = make(chan struct{})
	done := make(chan int, 1)
	go func() { done <- callService(s, "POST", "/api/conversations/"+c.ID+"/control", q).Code }()
	<-client.started
	// A switch after reservation is allowed and cannot retarget the accepted action.
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": c.Selected})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	close(client.release)
	if code := <-done; code != 200 {
		t.Fatal(code)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls != 1 || client.current != "test-model" {
		t.Fatal("reserved operation lost or repeated")
	}
}
func TestCanonicalControlUncertainDeliveryNeverRetriesOrLeaks(t *testing.T) {
	s, c := conditionalFixture(t)
	client := conditionalCounter(t, s)
	client.failure = true
	q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	path := "/api/conversations/" + c.ID + "/control"
	for range 3 {
		w := callService(s, "POST", path, q)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "uncertain") || strings.Contains(w.Body.String(), "private provider") {
			t.Fatal(w.Body.String())
		}
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls != 1 {
		t.Fatal("uncertain operation replayed")
	}
}
func TestCanonicalControlCapabilityLossBeforeCommit(t *testing.T) {
	s, c := conditionalFixture(t)
	client := conditionalCounter(t, s)
	q := frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
	client.mu.Lock()
	client.selectable = false
	client.mu.Unlock()
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "rejected") {
		t.Fatal(w.Body.String())
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls != 0 {
		t.Fatal("lost capability dispatched")
	}
}
func TestCanonicalControlCompetingManualDecisionAtMostOnce(t *testing.T) {
	s, c := conditionalFixture(t)
	m := s.sessions["Codex:"+testID]
	a := &approval{AgentSessionRequest: contract.AgentSessionRequest{ID: "request", Method: "permission.asked", Params: json.RawMessage(`{}`)}, Digest: "known", Origin: &controlOrigin{Conversation: c.ID, CommandID: "original-command"}, context: context.Background(), answer: make(chan json.RawMessage, 1)}
	s.convMu.Lock()
	current := s.conversations[c.ID]
	current.Commands = append(current.Commands, contract.AgentConversationCommand{ID: "original-command", Status: "running", Session: c.Selected})
	s.conversations[c.ID] = current
	s.convMu.Unlock()
	m.mu.Lock()
	m.requests[a.ID] = a
	m.mu.Unlock()
	q := prepareConditional(t, s, c, "request")
	q.RequestID = a.ID
	q.RequestDigest = a.Digest
	q.Result = json.RawMessage(`{"reply":"once"}`)
	frozen := frozenConditional(t, s, c, *q)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes <- callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen).Code
	}()
	go func() {
		defer wg.Done()
		codes <- callService(s, "POST", "/api/agents/codex-session/approve?conversation="+testID, map[string]any{"id": a.ID, "digest": a.Digest, "result": map[string]string{"reply": "reject"}}).Code
	}()
	wg.Wait()
	close(codes)
	success := 0
	for code := range codes {
		if code == 200 {
			success++
		}
	}
	if success != 1 || len(a.answer) != 1 {
		t.Fatalf("multiple or missing answers: successful=%d delivered=%d", success, len(a.answer))
	}
}

func TestCanonicalControlRestrictedDecisionShapes(t *testing.T) {
	for _, tc := range []struct{ method, params, result string }{
		{"item/commandExecution/requestApproval", `{}`, `{"decision":"accept","allowForSession":true}`},
		{"unknown/requestApproval", `{}`, `{"decision":"accept"}`},
		{"session/request_permission", `{"options":[{"optionId":"one","kind":"allow_once"}]}`, `{"outcome":{"outcome":"selected","optionId":"one","always":true}}`},
		{"item/tool/requestUserInput", `{"questions":[{"id":"q","isOther":true,"options":[{"label":"listed"}]}]}`, `{"answers":{"q":{"answers":["free form"]}}}`},
	} {
		if err := validateConditionalAnswer(contract.AgentSessionRequest{Method: tc.method, Params: json.RawMessage(tc.params)}, json.RawMessage(tc.result)); err == nil {
			t.Fatalf("unsupported answer accepted: %s %s", tc.method, tc.result)
		}
	}
}

func TestCanonicalControlStaleRequestCannotResolve(t *testing.T) {
	for _, stale := range []string{"completed-command", "changed-digest", "expired-context", "foreign-origin"} {
		t.Run(stale, func(t *testing.T) {
			s, c := conditionalFixture(t)
			m := s.sessions["Codex:"+testID]
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &approval{AgentSessionRequest: contract.AgentSessionRequest{ID: "pending", Method: "permission.asked", Params: json.RawMessage(`{}`)}, Digest: "original", Origin: &controlOrigin{Conversation: c.ID, CommandID: "command"}, context: ctx, answer: make(chan json.RawMessage, 1)}
			s.convMu.Lock()
			current := s.conversations[c.ID]
			current.Commands = append(current.Commands, contract.AgentConversationCommand{ID: "command", Status: "running", Session: c.Selected})
			if stale == "completed-command" {
				current.Commands[0].Status = "complete"
			}
			s.conversations[c.ID] = current
			s.convMu.Unlock()
			q := prepareConditional(t, s, c, "request")
			q.RequestID = a.ID
			q.RequestDigest = a.Digest
			q.Result = json.RawMessage(`{"reply":"once"}`)
			switch stale {
			case "changed-digest":
				q.RequestDigest = "stale"
			case "expired-context":
				cancel()
			case "foreign-origin":
				a.Origin.Conversation = "another"
			}
			m.mu.Lock()
			m.requests[a.ID] = a
			m.mu.Unlock()
			w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
			if w.Code == 200 {
				t.Fatal("stale request accepted")
			}
			if len(a.answer) != 0 {
				t.Fatal("stale request answered")
			}
			m.mu.Lock()
			kept := m.requests[a.ID] != nil
			m.mu.Unlock()
			if !kept {
				t.Fatal("conditional rejection removed manual request")
			}
		})
	}
}

func conditionalPendingFixture(t *testing.T, s *Server, c contract.AgentConversation) (canonicalControl, *approval) {
	t.Helper()
	m := s.sessions["Codex:"+testID]
	a := &approval{AgentSessionRequest: contract.AgentSessionRequest{ID: "pending-reservation", Method: "permission.asked", Params: json.RawMessage(`{}`)}, Digest: "original-digest", Origin: &controlOrigin{Conversation: c.ID, CommandID: "pending-command"}, context: context.Background(), answer: make(chan json.RawMessage, 1)}
	s.convMu.Lock()
	current := s.conversations[c.ID]
	current.Commands = append(current.Commands, contract.AgentConversationCommand{ID: a.Origin.CommandID, Status: "running", Session: c.Selected})
	s.conversations[c.ID] = current
	s.convMu.Unlock()
	m.mu.Lock()
	m.requests[a.ID] = a
	m.mu.Unlock()
	q := prepareConditional(t, s, c, "request")
	q.RequestID = a.ID
	q.RequestDigest = a.Digest
	q.Result = json.RawMessage(`{"reply":"once"}`)
	return frozenConditional(t, s, c, *q), a
}
func TestCanonicalControlPolicyRevokedBeforeReservation(t *testing.T) {
	for _, kind := range []string{"model", "request"} {
		for _, scope := range []string{"native", "conversation", "route"} {
			t.Run(kind+"/"+scope, func(t *testing.T) {
				s, c := conditionalFixture(t)
				client := conditionalCounter(t, s)
				var a *approval
				var q canonicalControl
				if kind == "model" {
					q = frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
				} else {
					q, a = conditionalPendingFixture(t, s, c)
				}
				var allowed atomic.Bool
				allowed.Store(true)
				checked := make(chan struct{})
				var once sync.Once
				s.cfg.Check = func(_ context.Context, obj, action string) error {
					match := false
					switch scope {
					case "native":
						match = strings.HasPrefix(obj, "agent/sessions/Codex/") && (action == "select-model" || action == "approve")
					case "conversation":
						match = obj == "agent/conversations/"+c.ID && action == "write"
					case "route":
						match = obj == "agent/sessions/api/conversations/"+c.ID+"/control" && action == "write"
					}
					if match {
						permit := allowed.Load()
						once.Do(func() { close(checked) })
						if !permit {
							return errors.New("revoked")
						}
					}
					return nil
				}
				s.convMu.Lock()
				done := make(chan int, 1)
				go func() { done <- callService(s, "POST", "/api/conversations/"+c.ID+"/control", q).Code }()
				select {
				case <-checked:
				case <-time.After(time.Second):
					s.convMu.Unlock()
					t.Fatal("initial policy not reached")
				}
				allowed.Store(false)
				s.convMu.Unlock()
				select {
				case code := <-done:
					if code == 200 {
						t.Fatal("revoked operation accepted")
					}
				case <-time.After(time.Second):
					t.Fatal("reservation hung")
				}
				client.mu.Lock()
				calls := client.calls
				client.mu.Unlock()
				if calls != 0 || a != nil && len(a.answer) != 0 {
					t.Fatal("revoked operation mutated native state")
				}
			})
		}
	}
}
func TestCanonicalControlExpiryAndCancellationWhileWaiting(t *testing.T) {
	for _, kind := range []string{"model", "request"} {
		for _, condition := range []string{"expired", "cancelled"} {
			t.Run(kind+"/"+condition, func(t *testing.T) {
				s, c := conditionalFixture(t)
				client := conditionalCounter(t, s)
				var a *approval
				var q canonicalControl
				if kind == "model" {
					q = frozenConditional(t, s, c, *prepareConditional(t, s, c, "model"))
				} else {
					q, a = conditionalPendingFixture(t, s, c)
				}
				if condition == "expired" {
					q.ExpiresAt = time.Now().Add(30 * time.Millisecond)
					q.Digest = controlDigest(c.ID, q)
					s.controls.mu.Lock()
					receipt := s.controls.receipts[q.ActionID]
					receipt.Proposal = q
					receipt.Digest = q.Digest
					s.controls.mu.Unlock()
				}
				checked := make(chan struct{})
				var once sync.Once
				s.cfg.Check = func(_ context.Context, obj, action string) error {
					if strings.HasPrefix(obj, "agent/sessions/Codex/") && (action == "select-model" || action == "approve") {
						once.Do(func() { close(checked) })
					}
					return nil
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				raw, _ := json.Marshal(q)
				r := httptest.NewRequest("POST", "http://127.0.0.1:3853/api/conversations/"+c.ID+"/control", bytes.NewReader(raw)).WithContext(ctx)
				r.Header.Set("Authorization", "Bearer "+s.token)
				s.convMu.Lock()
				done := make(chan int, 1)
				go func() { w := httptest.NewRecorder(); s.Handler().ServeHTTP(w, r); done <- w.Code }()
				select {
				case <-checked:
				case <-time.After(time.Second):
					s.convMu.Unlock()
					t.Fatal("initial check not reached")
				}
				if condition == "expired" {
					time.Sleep(time.Until(q.ExpiresAt) + time.Millisecond)
				} else {
					cancel()
				}
				s.convMu.Unlock()
				select {
				case code := <-done:
					if code == 200 {
						t.Fatal("expired/cancelled operation accepted")
					}
				case <-time.After(time.Second):
					t.Fatal("reservation hung")
				}
				client.mu.Lock()
				calls := client.calls
				client.mu.Unlock()
				if calls != 0 || a != nil && len(a.answer) != 0 {
					t.Fatal("stale operation mutated native state")
				}
			})
		}
	}
}
