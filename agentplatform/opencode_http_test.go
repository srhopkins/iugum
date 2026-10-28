package agentplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

type ocObserver struct{ text chan string }

func (o ocObserver) Event(k, v string) {
	if k == "text" {
		o.text <- v
	}
}
func (o ocObserver) Request(context.Context, contract.AgentSessionRequest) (json.RawMessage, error) {
	return json.RawMessage(`{"reply":"once"}`), nil
}
func TestOpenCodeHTTPTurn(t *testing.T) {
	events := make(chan string, 20)
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("directory") != "/work" {
			t.Error("directory lost")
		}
		switch r.URL.Path {
		case "/session/ses_test":
			fmt.Fprint(w, `{"id":"ses_test","directory":"/work"}`)
		case "/session/ses_test/message":
			fmt.Fprint(w, `[{"info":{"role":"user"},"parts":[{"type":"text","text":"old"}]}]`)
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
			posts.Add(1)
			var b struct {
				MessageID string `json:"messageID"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			w.WriteHeader(204)
			events <- `{"type":"session.idle","properties":{"sessionID":"ses_test"}}`
			events <- fmt.Sprintf(`{"type":"message.updated","properties":{"info":{"sessionID":"ses_test","id":"msg_answer","role":"assistant","parentID":%q}}}`, b.MessageID)
			events <- `{"type":"message.part.delta","properties":{"sessionID":"ses_test","messageID":"msg_answer","field":"text","delta":"hello"}}`
			events <- `{"type":"session.idle","properties":{"sessionID":"ses_test"}}`
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	o := ocObserver{make(chan string, 10)}
	s, err := (OpenCodeHTTPProvider{BaseURL: srv.URL}).Open(ctx, contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, o)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h, err := s.History(ctx)
	if err != nil || len(h) != 1 || h[0].Text != "old" {
		t.Fatalf("history %#v %v", h, err)
	}
	if err = s.Prompt(ctx, "hi"); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-o.text:
		if v != "hello" {
			t.Fatal(v)
		}
	default:
		t.Fatal("stale idle completed turn")
	}
	if posts.Load() != 1 {
		t.Fatal("replay")
	}
}
func TestOpenCodeHTTPRejectRemote(t *testing.T) {
	_, err := (OpenCodeHTTPProvider{BaseURL: "http://example.com"}).Open(context.Background(), contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, nil)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatal(err)
	}
}

func TestOpenCodeHTTPDisconnectNoReplay(t *testing.T) {
	var posts atomic.Int32
	disconnect := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/ses_test":
			fmt.Fprint(w, `{"id":"ses_test","directory":"/work"}`)
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			select {
			case <-disconnect:
			case <-r.Context().Done():
			}
		case "/session/ses_test/prompt_async":
			posts.Add(1)
			w.WriteHeader(204)
			close(disconnect)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s, e := (OpenCodeHTTPProvider{BaseURL: srv.URL}).Open(ctx, contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Prompt(ctx, "hello"); e == nil {
		t.Fatal("disconnect reported success")
	}
	if posts.Load() != 1 {
		t.Fatal("prompt replayed")
	}
}
func TestOpenCodeHTTPIdentityAndRedirect(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	for _, redirect := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, target.URL, 307)
			} else {
				fmt.Fprint(w, `{"id":"ses_other","directory":"/work"}`)
			}
		}))
		_, err := (OpenCodeHTTPProvider{BaseURL: srv.URL, Password: "secret"}).Open(context.Background(), contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, nil)
		srv.Close()
		if err == nil {
			t.Fatal("unsafe open succeeded")
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("credential leaked")
		}
	}
	if targetCalls.Load() != 0 {
		t.Fatal("followed redirect")
	}
}

type ocDecisionObserver struct {
	response json.RawMessage
	seen     chan contract.AgentSessionRequest
}

func (o ocDecisionObserver) Event(string, string) {}
func (o ocDecisionObserver) Request(_ context.Context, r contract.AgentSessionRequest) (json.RawMessage, error) {
	o.seen <- r
	return o.response, nil
}
func TestOpenCodeHTTPDecisionsScoped(t *testing.T) {
	for _, tc := range []struct {
		kind, response string
		want           bool
	}{{"permission.asked", `{"reply":"once"}`, true}, {"permission.asked", `{"reply":"invalid"}`, false}, {"question.asked", `{"answers":[["yes"]]}`, true}, {"question.asked", `{"answers":"yes"}`, false}} {
		t.Run(tc.kind+tc.response, func(t *testing.T) {
			replies := make(chan string, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("directory") != "/work" {
					t.Error("reply directory lost")
				}
				if u, p, ok := r.BasicAuth(); !ok || u != "opencode" || p != "secret" {
					t.Error("auth lost")
				}
				replies <- r.URL.Path
				fmt.Fprint(w, "true")
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			obs := ocDecisionObserver{json.RawMessage(tc.response), make(chan contract.AgentSessionRequest, 2)}
			s := &openCodeHTTPSession{p: OpenCodeHTTPProvider{BaseURL: srv.URL, Password: "secret"}, client: srv.Client(), ref: contract.AgentSessionRef{ID: "ses_test", Cwd: "/work"}, ctx: ctx, cancel: cancel, observer: obs, done: make(chan error, 1), turnCtx: ctx, requests: map[string]bool{}}
			s.handle([]byte(fmt.Sprintf(`{"type":%q,"properties":{"sessionID":"ses_foreign","id":"req_foreign"}}`, tc.kind)))
			select {
			case <-obs.seen:
				t.Fatal("foreign request surfaced")
			default:
			}
			s.reply(ctx, tc.kind, "req_own", json.RawMessage(`{"sessionID":"ses_test","questions":[{"question":"Continue?","options":[{"label":"yes"}],"custom":false}]}`))
			if r := <-obs.seen; r.ID != "req_own" || r.Method != tc.kind {
				t.Fatal(r)
			}
			select {
			case path := <-replies:
				if !tc.want || !strings.HasSuffix(path, "/req_own/reply") {
					t.Fatal(path)
				}
			default:
				if tc.want {
					t.Fatal("missing reply")
				}
			}
		})
	}
}
func TestOpenCodeHTTPAbortAndCancel(t *testing.T) {
	abort := make(chan struct{}, 1)
	posted := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/ses_test":
			fmt.Fprint(w, `{"id":"ses_test","directory":"/work"}`)
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/session/ses_test/prompt_async":
			posted <- struct{}{}
			w.WriteHeader(204)
		case "/session/ses_test/abort":
			abort <- struct{}{}
			fmt.Fprint(w, "true")
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	s, e := (OpenCodeHTTPProvider{BaseURL: srv.URL}).Open(context.Background(), contract.AgentSessionRef{Provider: "OpenCode", ID: "ses_test", Cwd: "/work"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- s.Prompt(ctx, "hi") }()
	<-posted
	if e = s.Interrupt(context.Background()); e != nil {
		t.Fatal(e)
	}
	<-abort
	cancel()
	select {
	case e := <-result:
		if e == nil {
			t.Fatal("cancellation reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not stop")
	}
}
