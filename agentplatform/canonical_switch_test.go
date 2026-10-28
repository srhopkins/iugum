package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func linkedSwitchFixture(t *testing.T) (*Server, contract.AgentConversation, contract.AgentSessionRef) {
	t.Helper()
	s, c := conditionalFixture(t)
	source := c.Selected
	target := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: source.Cwd, Title: "Saved target title"}
	for _, ref := range []contract.AgentSessionRef{target, source} {
		w := callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": ref})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
	}
	return s, c, target
}
func prepareSwitch(t *testing.T, s *Server, c contract.AgentConversation, target contract.AgentSessionRef) map[string]any {
	t.Helper()
	q := map[string]any{"phase": "prepare", "incarnation": s.controls.incarnation, "window": "exact-switch-window", "captureEpoch": 1, "actionId": operationFixtureID(), "revision": c.Revision, "session": c.Selected, "kind": "switch", "targetSession": target}
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
	if w.Code != 200 {
		t.Fatalf("prepare switch: %s", w.Body.String())
	}
	var frozen map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &frozen); err != nil {
		t.Fatal(err)
	}
	return frozen
}
func readSwitchConversation(t *testing.T, s *Server, id string) contract.AgentConversation {
	t.Helper()
	w := callService(s, "GET", "/api/conversations/"+id, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCanonicalSwitchPreparesExactExistingLinkWithoutMutation(t *testing.T) {
	s, c, target := linkedSwitchFixture(t)
	candidate := target
	candidate.Title = "Untrusted client title"
	frozen := prepareSwitch(t, s, c, candidate)
	got := frozen["targetSession"].(map[string]any)
	if got["sessionId"] != target.ID || got["title"] != target.Title {
		t.Fatal("target was not resolved from saved canonical link")
	}
	source := frozen["session"].(map[string]any)
	if source["sessionId"] != c.Selected.ID {
		t.Fatal("source target changed")
	}
	after := readSwitchConversation(t, s, c.ID)
	if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID || len(s.sessions) != 2 {
		t.Fatal("preparation mutated/opened native session")
	}
}
func TestCanonicalSwitchCommitsOnceAndPreservesBusyOwners(t *testing.T) {
	s, c, target := linkedSwitchFixture(t)
	sourceOwner, targetOwner := s.sessions["Codex:"+c.Selected.ID], s.sessions["Codex:"+target.ID]
	for _, m := range []*managed{sourceOwner, targetOwner} {
		m.mu.Lock()
		m.busy = true
		m.state = "Working"
		m.mu.Unlock()
	}
	frozen := prepareSwitch(t, s, c, target)
	frozen["phase"] = "commit"
	path := "/api/conversations/" + c.ID + "/control"
	for range 2 {
		w := callService(s, "POST", path, frozen)
		if w.Code != 200 {
			t.Fatalf("commit: %s", w.Body.String())
		}
		var result map[string]string
		json.Unmarshal(w.Body.Bytes(), &result)
		if result["status"] != "completed" {
			t.Fatal(w.Body.String())
		}
	}
	after := readSwitchConversation(t, s, c.ID)
	if after.Selected.ID != target.ID || after.Revision != c.Revision+1 || len(after.Links) != len(c.Links) || len(after.Messages) != len(c.Messages) {
		t.Fatal("switch state lost or repeated")
	}
	if s.sessions["Codex:"+c.Selected.ID] != sourceOwner || s.sessions["Codex:"+target.ID] != targetOwner || len(s.sessions) != 2 {
		t.Fatal("native owner changed")
	}
	for _, m := range []*managed{sourceOwner, targetOwner} {
		m.mu.Lock()
		busy := m.busy
		m.mu.Unlock()
		client := m.client.(*controlsSession).AgentSession.(*testSession)
		client.mu.Lock()
		closed, calls := client.closed, client.calls
		client.mu.Unlock()
		if !busy || closed || calls != 0 || len(client.interrupted) != 0 {
			t.Fatal("background owner interrupted, closed or prompted")
		}
	}
}

func TestCanonicalSwitchRejectsUnlinkedOrUnavailableWithoutOpen(t *testing.T) {
	for _, condition := range []string{"unlinked", "wrong-cwd", "missing-target-owner", "missing-source-owner", "target-not-ready"} {
		t.Run(condition, func(t *testing.T) {
			s, c, target := linkedSwitchFixture(t)
			before := len(s.sessions)
			switch condition {
			case "unlinked":
				target.ID = "33333333-3333-4333-8333-333333333333"
			case "wrong-cwd":
				target.Cwd = t.TempDir()
			case "missing-target-owner":
				s.mu.Lock()
				delete(s.sessions, "Codex:"+target.ID)
				s.mu.Unlock()
				before--
			case "missing-source-owner":
				s.mu.Lock()
				delete(s.sessions, "Codex:"+c.Selected.ID)
				s.mu.Unlock()
				before--
			case "target-not-ready":
				m := s.sessions["Codex:"+target.ID]
				m.mu.Lock()
				m.ready = false
				m.mu.Unlock()
			}
			q := map[string]any{"phase": "prepare", "incarnation": s.controls.incarnation, "window": "window", "actionId": operationFixtureID(), "revision": c.Revision, "session": c.Selected, "kind": "switch", "targetSession": target}
			w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", q)
			if w.Code == 200 {
				t.Fatal("invalid target accepted")
			}
			if len(s.sessions) != before {
				t.Fatal("invalid switch opened owner")
			}
			after := readSwitchConversation(t, s, c.ID)
			if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID || len(after.Links) != 2 {
				t.Fatal("invalid preparation changed canonical state")
			}
		})
	}
}
func TestCanonicalSwitchBindingAndOneTimeIdentity(t *testing.T) {
	for _, field := range []string{"source", "target", "window", "epoch", "revision", "digest", "incarnation", "model"} {
		t.Run(field, func(t *testing.T) {
			s, c, target := linkedSwitchFixture(t)
			frozen := prepareSwitch(t, s, c, target)
			frozen["phase"] = "commit"
			switch field {
			case "source":
				frozen["session"].(map[string]any)["sessionId"] = target.ID
			case "target":
				frozen["targetSession"].(map[string]any)["cwd"] = t.TempDir()
			case "window":
				frozen["window"] = "other"
			case "epoch":
				frozen["captureEpoch"] = 99
			case "revision":
				frozen["revision"] = c.Revision + 1
			case "digest":
				frozen["digest"] = "wrong"
			case "incarnation":
				frozen["incarnation"] = "old"
			case "model":
				frozen["model"] = "not-a-model-control"
			}
			if w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen); w.Code == 200 {
				t.Fatal("changed switch binding accepted")
			}
			after := readSwitchConversation(t, s, c.ID)
			if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID {
				t.Fatal("changed binding mutated selection")
			}
		})
	}
}
func TestCanonicalSwitchRestartRejectsLateOriginalWithoutOpen(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(strconv.FormatBool(accepted), func(t *testing.T) {
			s, c, target := linkedSwitchFixture(t)
			frozen := prepareSwitch(t, s, c, target)
			frozen["phase"] = "commit"
			path := "/api/conversations/" + c.ID + "/control"
			if accepted {
				if w := callService(s, "POST", path, frozen); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
			s.Close()
			next, err := New(context.Background(), s.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Close()
			if w := callService(next, "POST", path, frozen); w.Code == 200 {
				t.Fatal("late prior-incarnation switch accepted")
			}
			if len(next.sessions) != 0 {
				t.Fatal("late switch opened native sessions")
			}
			current := readSwitchConversation(t, next, c.ID)
			expected, revision := c.Selected.ID, c.Revision
			if accepted {
				expected = target.ID
				revision++
			}
			if current.Selected.ID != expected || current.Revision != revision {
				t.Fatal("restart late request changed persisted selection")
			}
		})
	}
}
func TestCanonicalSwitchPersistenceUncertainNeverReplays(t *testing.T) {
	s, c, target := linkedSwitchFixture(t)
	frozen := prepareSwitch(t, s, c, target)
	frozen["phase"] = "commit"
	original := s.cfg.DataDir
	s.cfg.DataDir = filepath.Join(t.TempDir(), "missing")
	w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "uncertain") {
		t.Fatal(w.Body.String())
	}
	s.cfg.DataDir = original
	w = callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "uncertain") {
		t.Fatal("uncertain save replayed")
	}
	after := readSwitchConversation(t, s, c.ID)
	if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID || s.selected["Codex"] != c.Selected.ID {
		t.Fatal("failed save changed selection")
	}
}
func TestCanonicalSwitchPolicyRevokedAtReservation(t *testing.T) {
	for _, scope := range []string{"source", "target", "conversation", "route"} {
		t.Run(scope, func(t *testing.T) {
			s, c, target := linkedSwitchFixture(t)
			frozen := prepareSwitch(t, s, c, target)
			frozen["phase"] = "commit"
			var allowed atomic.Bool
			allowed.Store(true)
			checked := make(chan struct{})
			var once sync.Once
			s.cfg.Check = func(_ context.Context, obj, action string) error {
				match := false
				switch scope {
				case "source":
					match = obj == "agent/sessions/Codex/"+c.Selected.ID && action == "select"
				case "target":
					match = obj == "agent/sessions/Codex/"+target.ID && action == "select"
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
			go func() { done <- callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen).Code }()
			select {
			case <-checked:
			case <-time.After(time.Second):
				s.convMu.Unlock()
				t.Fatal("initial gate not reached")
			}
			allowed.Store(false)
			s.convMu.Unlock()
			select {
			case code := <-done:
				if code == 200 {
					t.Fatal("revoked switch accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("switch did not settle")
			}
			after := readSwitchConversation(t, s, c.ID)
			if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID {
				t.Fatal("revoked switch mutated selection")
			}
		})
	}
}
func TestCanonicalSwitchExpiresOrCancelsWhileWaiting(t *testing.T) {
	for _, condition := range []string{"expiry", "cancellation"} {
		t.Run(condition, func(t *testing.T) {
			s, c, target := linkedSwitchFixture(t)
			frozen := prepareSwitch(t, s, c, target)
			frozen["phase"] = "commit"
			raw, _ := json.Marshal(frozen)
			var typed canonicalControl
			json.Unmarshal(raw, &typed)
			if condition == "expiry" {
				typed.ExpiresAt = time.Now().Add(30 * time.Millisecond)
				typed.Digest = controlDigest(c.ID, typed)
				s.controls.mu.Lock()
				receipt := s.controls.receipts[typed.ActionID]
				receipt.Proposal = typed
				receipt.Digest = typed.Digest
				s.controls.mu.Unlock()
			}
			raw, _ = json.Marshal(typed)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("POST", "http://127.0.0.1:3853/api/conversations/"+c.ID+"/control", bytes.NewReader(raw)).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+s.token)
			checked := make(chan struct{})
			var once sync.Once
			s.cfg.Check = func(_ context.Context, obj, action string) error {
				if obj == "agent/sessions/Codex/"+c.Selected.ID && action == "select" {
					once.Do(func() { close(checked) })
				}
				return nil
			}
			s.convMu.Lock()
			done := make(chan int, 1)
			go func() { w := httptest.NewRecorder(); s.Handler().ServeHTTP(w, r); done <- w.Code }()
			select {
			case <-checked:
			case <-time.After(time.Second):
				s.convMu.Unlock()
				t.Fatal("initial gate not reached")
			}
			if condition == "expiry" {
				time.Sleep(time.Until(typed.ExpiresAt) + time.Millisecond)
			} else {
				cancel()
			}
			s.convMu.Unlock()
			select {
			case code := <-done:
				if code == 200 {
					t.Fatal("stale switch accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("switch did not settle")
			}
			after := readSwitchConversation(t, s, c.ID)
			if after.Revision != c.Revision || after.Selected.ID != c.Selected.ID {
				t.Fatal("stale switch mutated selection")
			}
		})
	}
}

type namedSwitchProvider struct {
	*testProvider
	name string
}

func (p namedSwitchProvider) Name() string { return p.name }
func TestCanonicalSwitchSameNativeIDAcrossProvidersStaysExact(t *testing.T) {
	providers := []contract.AgentSessionProvider{namedSwitchProvider{&testProvider{clients: map[string]*testSession{}}, "Codex"}, namedSwitchProvider{&testProvider{clients: map[string]*testSession{}}, "Claude"}}
	s, err := New(context.Background(), Config{DataDir: t.TempDir(), Providers: providers})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := contract.AgentSessionRef{Provider: "Codex", ID: testID, Cwd: t.TempDir()}
	target := source
	target.Provider = "Claude"
	target.Title = "Claude exact target"
	w := callService(s, "POST", "/api/conversations", map[string]any{"session": source})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var c contract.AgentConversation
	json.Unmarshal(w.Body.Bytes(), &c)
	for _, ref := range []contract.AgentSessionRef{target, source} {
		w = callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": ref})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &c)
	}
	frozen := prepareSwitch(t, s, c, target)
	frozen["phase"] = "commit"
	if w = callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	after := readSwitchConversation(t, s, c.ID)
	if after.Selected.Provider != "Claude" || after.Selected.ID != testID || len(s.sessions) != 2 || s.sessions["Claude:"+testID] == s.sessions["Codex:"+testID] {
		t.Fatal("same-ID providers were conflated")
	}
}

func TestCanonicalSwitchActualBackgroundCompletionsRemainIndependent(t *testing.T) {
	s, c, target := linkedSwitchFixture(t)
	source := c.Selected
	switchTo := func(ref contract.AgentSessionRef) {
		t.Helper()
		w := callService(s, "POST", "/api/conversations/"+c.ID+"/switch", map[string]any{"revision": c.Revision, "session": ref})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		json.Unmarshal(w.Body.Bytes(), &c)
	}
	start := func(ref contract.AgentSessionRef) *testSession {
		t.Helper()
		w := callService(s, "POST", "/api/conversations/"+c.ID+"/chat", map[string]any{"revision": c.Revision, "session": ref, "text": "synthetic background turn", "commandId": operationFixtureID()})
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		client := s.sessions[ref.Provider+":"+ref.ID].client.(*controlsSession).AgentSession.(*testSession)
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("fake background turn never started")
		}
		return client
	}
	switchTo(target)
	targetClient := start(target)
	switchTo(source)
	sourceClient := start(source)
	c = readSwitchConversation(t, s, c.ID)
	frozen := prepareSwitch(t, s, c, target)
	frozen["phase"] = "commit"
	if w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	selectedRevision := c.Revision + 1
	if len(sourceClient.interrupted) != 0 || len(targetClient.interrupted) != 0 {
		t.Fatal("switch interrupted background owner")
	}
	close(sourceClient.finish)
	close(targetClient.finish)
	complete := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		after := readSwitchConversation(t, s, c.ID)
		if after.Selected.ID != target.ID || after.Revision != selectedRevision {
			t.Fatal("background completion overwrote selected target")
		}
		count := 0
		for _, cmd := range after.Commands {
			if cmd.Status == "complete" {
				count++
			}
		}
		if count == 2 {
			complete = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !complete {
		t.Fatal("independent background turns did not complete")
	}
	for _, client := range []*testSession{sourceClient, targetClient} {
		client.mu.Lock()
		calls, closed := client.calls, client.closed
		client.mu.Unlock()
		if calls != 1 || closed || len(client.interrupted) != 0 {
			t.Fatal("background owner replayed or disposed")
		}
	}
}
func TestCanonicalSwitchConcurrentDuplicateReservesOneRevision(t *testing.T) {
	s, c, target := linkedSwitchFixture(t)
	frozen := prepareSwitch(t, s, c, target)
	frozen["phase"] = "commit"
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			w := callService(s, "POST", "/api/conversations/"+c.ID+"/control", frozen)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "completed") {
				t.Errorf("duplicate failed: %s", w.Body.String())
			}
		}()
	}
	group.Wait()
	after := readSwitchConversation(t, s, c.ID)
	if after.Revision != c.Revision+1 || after.Selected.ID != target.ID {
		t.Fatal("concurrent delivery repeated canonical mutation")
	}
}
