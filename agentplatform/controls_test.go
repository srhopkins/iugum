package agentplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/contract"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type controlsProvider struct{ *testProvider }

func (p controlsProvider) Create(ctx context.Context, q contract.AgentSessionCreate, o contract.AgentSessionObserver) (contract.AgentSessionRef, contract.AgentSession, error) {
	ref := contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: q.Cwd, Title: q.Title}
	s, e := p.Open(ctx, ref, o)
	return ref, s, e
}

func (p controlsProvider) Open(ctx context.Context, ref contract.AgentSessionRef, o contract.AgentSessionObserver) (contract.AgentSession, error) {
	client, err := p.testProvider.Open(ctx, ref, o)
	if err != nil {
		return nil, err
	}
	return &controlsSession{AgentSession: client}, nil
}

type controlsSession struct {
	contract.AgentSession
	model string
}

func (s *controlsSession) Models(context.Context) (contract.AgentSessionModels, error) {
	return contract.AgentSessionModels{Options: []contract.AgentSessionModel{{ID: "test-model", Name: "Test"}}, Current: s.model, CanSelect: true}, nil
}
func (s *controlsSession) SelectModel(_ context.Context, id string) error {
	if id != "test-model" {
		return errors.New("unknown model")
	}
	s.model = id
	return nil
}
func TestSessionControlsCreateAndModels(t *testing.T) {
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	q := map[string]string{"provider": "Codex", "cwd": t.TempDir(), "title": "New"}
	if r := callService(s, "POST", "/api/sessions", q); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	path := "/api/agents/codex-session/models?conversation=" + otherID
	if r := callService(s, "POST", path, map[string]string{"model": "test-model"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := callService(s, "POST", path, map[string]string{"model": "bad"}); r.Code == 200 {
		t.Fatal("unknown model accepted")
	}
	s.sessions["Codex:"+otherID].busy = true
	if r := callService(s, "POST", path, map[string]string{"model": "test-model"}); r.Code == 200 {
		t.Fatal("busy model changed")
	}
}
func TestSessionControlsUnsupportedDoesNotCreate(t *testing.T) {
	s, p := testServer(t)
	if r := callService(s, "GET", "/api/agents/claude-session/conversation", nil); r.Code == 200 {
		t.Fatal("unconfigured provider accepted")
	}
	if r := callService(s, "POST", "/api/sessions", map[string]string{"provider": "Codex", "cwd": t.TempDir()}); r.Code == 200 {
		t.Fatal("unsupported create")
	}
	if len(p.clients) != 0 {
		t.Fatal("opened session")
	}
}

func TestSessionControlsProtocolCreateAndModels(t *testing.T) {
	for _, name := range []string{"Codex", "Cursor"} {
		t.Run(name, func(t *testing.T) {
			p := Provider{Provider: name, Binary: helperBinary(t, "")}
			ref, client, err := p.Create(context.Background(), contract.AgentSessionCreate{Provider: name, Cwd: t.TempDir()}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if ref.ID != testSessionID {
				t.Fatal(ref)
			}
			c := client.(contract.AgentSessionModelController)
			models, err := c.Models(context.Background())
			if err != nil || len(models.Options) != 1 {
				t.Fatalf("%+v %v", models, err)
			}
			if err = c.SelectModel(context.Background(), "fixture-model"); err != nil {
				t.Fatal(err)
			}
			if err = c.SelectModel(context.Background(), "unknown"); err == nil {
				t.Fatal("unknown model accepted")
			}
		})
	}
}

func TestSessionControlsPolicyAndPersistence(t *testing.T) {
	p := controlsProvider{&testProvider{clients: map[string]*testSession{}}}
	dir := t.TempDir()
	work := t.TempDir()
	deny := "create"
	s, e := New(context.Background(), Config{DataDir: dir, Providers: []contract.AgentSessionProvider{p}, Check: func(_ context.Context, _ string, act string) error {
		if act == deny {
			return errors.New("denied")
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	q := map[string]string{"provider": "Codex", "cwd": work}
	if r := callService(s, "POST", "/api/sessions", q); r.Code == 200 {
		t.Fatal("create policy bypass")
	}
	deny = ""
	if r := callService(s, "POST", "/api/sessions", q); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	path := "/api/agents/codex-session/models?conversation=" + otherID
	deny = "select-model"
	if r := callService(s, "POST", path, map[string]string{"model": "test-model"}); r.Code == 200 {
		t.Fatal("model policy bypass")
	}
	deny = ""
	if r := callService(s, "POST", path, map[string]string{"model": "test-model"}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	b, e := os.ReadFile(filepath.Join(dir, "model-Codex-"+otherID+".json"))
	if e != nil || !bytes.Contains(b, []byte("test-model")) {
		t.Fatalf("model persistence %s %v", b, e)
	}
	s2, e := New(context.Background(), Config{DataDir: dir, Providers: []contract.AgentSessionProvider{p}})
	if e != nil {
		t.Fatal(e)
	}
	defer s2.Close()
	if r := callService(s2, "POST", "/api/select", contract.AgentSessionRef{Provider: "Codex", ID: otherID, Cwd: work}); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := callService(s2, "GET", path, nil); r.Code != 200 || !bytes.Contains(r.Body.Bytes(), []byte(`"current":"test-model"`)) {
		t.Fatal(r.Body.String())
	}
}
func TestSessionControlsOpenCodeCreateAndModels(t *testing.T) {
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/provider":
			fmt.Fprint(w, `{"all":[{"id":"connected","models":{"fixture":{"name":"Fixture"}}},{"id":"offline","models":{"bad":{"name":"Bad"}}}],"connected":["connected"]}`)
		case "/session":
			creates.Add(1)
			fmt.Fprint(w, `{"id":"ses_created","directory":"/work"}`)
		case "/session/ses_created":
			fmt.Fprint(w, `{"id":"ses_created","directory":"/work"}`)
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := OpenCodeHTTPProvider{BaseURL: srv.URL}
	q := contract.AgentSessionCreate{Provider: "OpenCode", Cwd: "/work", Model: "offline/bad"}
	if _, _, e := p.Create(context.Background(), q, nil); e == nil || creates.Load() != 0 {
		t.Fatal("unknown model created session")
	}
	q.Model = "connected/fixture"
	ref, c, e := p.Create(context.Background(), q, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if ref.ID != "ses_created" {
		t.Fatal(ref)
	}
	models, e := c.(contract.AgentSessionModelController).Models(context.Background())
	if e != nil || models.Current != q.Model || len(models.Options) != 1 {
		t.Fatalf("%+v %v", models, e)
	}
}

type approvalProvider struct {
	*testProvider
	name string
}

func (p approvalProvider) Name() string { return p.name }
func TestSessionControlsNativeDecisionsThroughHTTP(t *testing.T) {
	cases := []struct{ name, method, params, answer, bad string }{
		{"Cursor", "cursor/create_plan", `{"toolCallId":"plan1","plan":"Do work"}`, `{"outcome":{"outcome":"accepted"}}`, `{"outcome":"accepted"}`},
		{"Cursor", "cursor/ask_question", `{"questions":[{"id":"q1","options":[{"id":"one"}]}]}`, `{"outcome":{"outcome":"answered","answers":[{"questionId":"q1","selectedOptionIds":["one"]}]}}`, `{"outcome":{"outcome":"answered","answers":[]}}`},
		{"Codex", "item/commandExecution/requestApproval", `{"availableDecisions":["decline","cancel"]}`, `{"decision":"decline"}`, `{"decision":"accept"}`},
		{"Codex", "item/tool/requestUserInput", `{"questions":[{"id":"q1","options":[{"label":"One"}],"isOther":false}]}`, `{"answers":{"q1":{"answers":["One"]}}}`, `{"answers":{"wrong":{"answers":["One"]}}}`},
		{"Claude", "can_use_tool", `{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"pwd"}}`, `{"behavior":"allow","updatedInput":{"command":"pwd"}}`, `{"behavior":"allow"}`},
		{"Claude", "can_use_tool", `{"subtype":"can_use_tool","tool_name":"Bash","input":{}}`, `{"behavior":"deny","message":"No"}`, `{"behavior":"auto"}`},
		{"OpenCode", "permission.asked", `{"id":"p1","sessionID":"ses_test"}`, `{"reply":"once"}`, `{"reply":"always"}`},
		{"OpenCode", "question.asked", `{"id":"q1","questions":[{"question":"Pick","options":[{"label":"Choice"}],"custom":false}]}`, `{"answers":[["Choice"]]}`, `{"answers":[["Other"]]}`},
		{"OpenCode", "question.asked", `{"id":"q1","questions":[{"question":"Pick","options":[{"label":"Choice"}]}]}`, `{"reject":true}`, `{"reject":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name+tc.method+tc.answer, func(t *testing.T) {
			p := approvalProvider{&testProvider{clients: map[string]*testSession{}}, tc.name}
			s, e := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{p}})
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			id := testID
			if tc.name == "OpenCode" {
				id = "ses_test"
			}
			ref := contract.AgentSessionRef{Provider: tc.name, ID: id, Cwd: t.TempDir()}
			if r := callService(s, "POST", "/api/select", ref); r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan json.RawMessage, 1)
			go func() {
				answer, _ := p.clients[id].observer.Request(ctx, contract.AgentSessionRequest{Method: tc.method, Params: json.RawMessage(tc.params)})
				done <- answer
			}()
			path := "/api/agents/" + providerRoute(tc.name) + "/"
			var pending []approval
			for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
				r := callService(s, "GET", path+"approvals?conversation="+id, nil)
				_ = json.Unmarshal(r.Body.Bytes(), &pending)
				if len(pending) > 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if len(pending) != 1 {
				t.Fatal("no pending decision")
			}
			body := map[string]any{"id": pending[0].ID, "digest": pending[0].Digest, "result": json.RawMessage(tc.bad)}
			if r := callService(s, "POST", path+"approve?conversation="+id, body); r.Code == 200 {
				t.Fatal("invalid native decision accepted")
			}
			body["result"] = json.RawMessage(tc.answer)
			if r := callService(s, "POST", path+"approve?conversation="+id, body); r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			if got := <-done; string(got) != tc.answer {
				t.Fatalf("native response altered: %s", got)
			}
		})
	}
}

func TestSessionControlsClaudeSavedDefaultReload(t *testing.T) {
	dir := t.TempDir()
	ref := contract.AgentSessionRef{Provider: "Claude", ID: testID, Cwd: t.TempDir()}
	m := &managed{ref: ref, client: &claudeSession{provider: ClaudeProvider{Model: "configured"}}}
	s, e := New(context.Background(), Config{DataDir: dir})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	saved, _ := json.Marshal(map[string]any{"ref": ref, "model": "configured"})
	if e = os.WriteFile(filepath.Join(dir, "model-Claude-"+testID+".json"), saved, 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.restoreModel(context.Background(), m); e != nil {
		t.Fatal(e)
	}
}
func TestSessionControlsNativeChoiceShapes(t *testing.T) {
	tests := []struct{ method, params, valid, invalid string }{
		{"item/commandExecution/requestApproval", `{"availableDecisions":["decline","cancel"]}`, `{"decision":"decline"}`, `{"decision":"accept"}`},
		{"cursor/ask_question", `{"questions":[{"id":"q1","options":[{"id":"one"}]}]}`, `{"outcome":{"outcome":"answered","answers":[{"questionId":"q1","selectedOptionIds":["one"]}]}}`, `{"outcome":{"outcome":"answered","answers":[]}}`},
		{"item/tool/requestUserInput", `{"questions":[{"id":"q1","options":[{"label":"One"}],"isOther":false}]}`, `{"answers":{"q1":{"answers":["One"]}}}`, `{"answers":{"wrong":{"answers":["One"]}}}`},
	}
	for _, tc := range tests {
		q := contract.AgentSessionRequest{Method: tc.method, Params: json.RawMessage(tc.params)}
		if e := validateAnswer(q, json.RawMessage(tc.valid)); e != nil {
			t.Errorf("%s valid: %v", tc.method, e)
		}
		if e := validateAnswer(q, json.RawMessage(tc.invalid)); e == nil {
			t.Errorf("%s invalid accepted", tc.method)
		}
	}
}

func TestSessionControlsOpenCodeQuestionRejectAndExpiryHTTP(t *testing.T) {
	events := make(chan string, 20)
	rejected := make(chan string, 1)
	var requests atomic.Int32
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/ses_test":
			fmt.Fprint(w, `{"id":"ses_test","directory":"/work"}`)
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			for {
				select {
				case e := <-events:
					fmt.Fprintf(w, "data: %s\n\n", e)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		case "/session/ses_test/prompt_async":
			var b struct {
				ID string `json:"messageID"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			requests.Add(1)
			w.WriteHeader(204)
			events <- fmt.Sprintf(`{"type":"message.updated","properties":{"info":{"sessionID":"ses_test","id":"answer","role":"assistant","parentID":%q}}}`, b.ID)
			events <- `{"type":"question.asked","properties":{"sessionID":"ses_test","id":"q1","questions":[{"question":"Pick","options":[{"label":"One"}],"custom":false}]}}`
		case "/question/q1/reject":
			rejected <- r.URL.Path
			w.WriteHeader(204)
			events <- `{"type":"session.idle","properties":{"sessionID":"ses_test"}}`
		default:
			t.Errorf("unexpected native request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer native.Close()
	s, e := New(context.Background(), Config{DataDir: t.TempDir(), Providers: []contract.AgentSessionProvider{OpenCodeHTTPProvider{BaseURL: native.URL}}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	// Synthetic provider directory need not exist for adapter; service requires it.
	p := OpenCodeHTTPProvider{BaseURL: native.URL}
	m := &managed{ref: contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, ready: true, done: s.ctx, requests: map[string]*approval{}}
	c, e := p.Open(s.ctx, m.ref, m)
	if e != nil {
		t.Fatal(e)
	}
	m.client = c
	s.sessions["OpenCode:ses_test"] = m
	s.selected["OpenCode"] = "ses_test"
	path := "/api/agents/opencode-session/"
	prompt := func(id string) chan *httptest.ResponseRecorder {
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			done <- callService(s, "POST", path+"chat?conversation=ses_test", map[string]string{"text": "work", "commandId": id})
		}()
		return done
	}
	pending := func() approval {
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
			var a []approval
			r := callService(s, "GET", path+"approvals?conversation=ses_test", nil)
			_ = json.Unmarshal(r.Body.Bytes(), &a)
			if len(a) > 0 {
				return a[0]
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("no native question")
		return approval{}
	}
	done := prompt(testID)
	a := pending()
	body := map[string]any{"id": a.ID, "digest": a.Digest, "result": map[string]bool{"reject": true}}
	if r := callService(s, "POST", path+"approve?conversation=ses_test", body); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	select {
	case <-rejected:
	case <-time.After(time.Second):
		t.Fatal("question reject was not sent to native endpoint")
	}
	if r := <-done; r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	done = prompt(otherID)
	a = pending()
	events <- `{"type":"session.idle","properties":{"sessionID":"ses_test"}}`
	if r := <-done; r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	body["id"] = a.ID
	body["digest"] = a.Digest
	if r := callService(s, "POST", path+"approve?conversation=ses_test", body); r.Code == 200 {
		t.Fatal("expired native question accepted")
	}
	if requests.Load() != 2 {
		t.Fatal("unexpected prompt replay")
	}
}

func TestSessionControlsCodexOriginalDirectory(t *testing.T) {
	for _, mode := range []string{"wrongcwd", "resumecwd"} {
		t.Run(mode, func(t *testing.T) {
			p := Provider{Provider: "Codex", Binary: helperBinary(t, mode)}
			c, e := p.Open(context.Background(), contract.AgentSessionRef{Provider: "Codex", ID: testSessionID, Cwd: t.TempDir()}, nil)
			if mode == "wrongcwd" {
				if e == nil {
					c.Close()
					t.Fatal("foreign original directory accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if e = c.Prompt(context.Background(), "work"); e == nil || !strings.Contains(e.Error(), "directory") {
				t.Fatalf("mismatched resumed directory: %v", e)
			}
		})
	}
}

func TestSessionControlsOpenCodeStopBeforeAssistant(t *testing.T) {
	aborted := make(chan struct{}, 1)
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/ses_test/abort" {
			t.Error(r.URL.Path)
		}
		aborted <- struct{}{}
		w.WriteHeader(200)
	}))
	defer native.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	turn, stop := context.WithCancel(ctx)
	s := &openCodeHTTPSession{p: OpenCodeHTTPProvider{BaseURL: native.URL}, client: native.Client(), ref: contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, ctx: ctx, cancel: cancel, turnCtx: turn, turnCancel: stop, done: make(chan error, 1), assistants: map[string]bool{}}
	s.handle([]byte(`{"type":"session.idle","properties":{"sessionID":"ses_test"}}`))
	select {
	case <-s.done:
		t.Fatal("unsolicited idle settled new turn")
	default:
	}
	if e := s.Interrupt(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-aborted
	s.handle([]byte(`{"type":"session.idle","properties":{"sessionID":"ses_test"}}`))
	select {
	case <-s.done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("confirmed stop before assistant left turn busy")
	}
}
