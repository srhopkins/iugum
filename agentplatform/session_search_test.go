package agentplatform

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/agentsessions"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionSearchIndexesReadOnlyWithoutOpeningProviders(t *testing.T) {
	s, p := testServer(t)
	root := filepath.Join(s.cfg.Home, ".codex", "sessions")
	os.MkdirAll(root, 0700)
	os.WriteFile(filepath.Join(root, "test.jsonl"), []byte(`{"type":"session_meta","payload":{"id":"same-id","cwd":"/project"}}
{"type":"response_item","payload":{"type":"message","content":[{"type":"input_text","text":"First title"}]}}
{"type":"response_item","payload":{"type":"message","content":[{"type":"output_text","text":"content-only needle"}]}}
`), 0600)
	w := callService(s, "GET", "/api/session-search?q=needle", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Sessions []struct {
			ID string `json:"id"`
		}
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Sessions) != 1 {
		t.Fatal(w.Body.String(), err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("missing source coverage warning")
	}
	w = callService(s, "GET", "/api/session-transcript?id="+result.Sessions[0].ID, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "content-only needle") {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(p.clients) != 0 || len(s.conversations) != 0 {
		t.Fatal("read-only search opened provider or conversation")
	}
}

func TestSessionSearchDeniedSourceNeverIngested(t *testing.T) {
	s, _ := testServer(t)
	root := filepath.Join(s.cfg.Home, ".codex", "sessions")
	os.MkdirAll(root, 0700)
	os.WriteFile(filepath.Join(root, "denied.jsonl"), []byte(`{"content":"private denied transcript"}`), 0600)
	s.cfg.Check = func(_ context.Context, resource, action string) error {
		if strings.HasPrefix(resource, "agent/sessions/transcript:codex:") {
			return errors.New("denied")
		}
		return nil
	}
	w := callService(s, "GET", "/api/session-search?q=private", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private denied") {
		t.Fatal(w.Code, w.Body.String())
	}
	index, err := agentsessions.Open(filepath.Join(s.cfg.DataDir, "transcript-search.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	rows, err := index.Sessions(context.Background(), "", func(agentsessions.Hit) error { return nil })
	if err != nil || len(rows) != 0 {
		t.Fatal("denied source persisted", rows, err)
	}
}

func TestSessionSearchCustomTitleAndRemoteOpenCode(t *testing.T) {
	s, p := testServer(t)
	claudeRoot := filepath.Join(s.cfg.Home, ".claude")
	project := filepath.Join(claudeRoot, "projects", "project")
	os.MkdirAll(project, 0700)
	os.WriteFile(filepath.Join(project, testID+".jsonl"), []byte(`{"type":"user","cwd":"/project","sessionId":"`+testID+`","message":{"content":"content without title"}}
{"type":"custom-title","customTitle":"unicorn title"}
`), 0600)
	s.providers["Claude"] = ClaudeProvider{ConfigDir: claudeRoot}
	w := callService(s, "GET", "/api/session-search?q=unicorn", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "unicorn title") {
		t.Fatal(w.Code, w.Body.String())
	}
	methods := []string{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		switch r.URL.Path {
		case "/session":
			w.Write([]byte(`[{"id":"ses_remote","directory":"/remote","title":"pegasus title"}]`))
		case "/session/ses_remote/message":
			w.Write([]byte(`[{"info":{"role":"assistant"},"parts":[{"type":"text","text":"remote-only body"}]}]`))
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	s.providers["OpenCode"] = OpenCodeHTTPProvider{BaseURL: api.URL}
	for _, query := range []string{"pegasus", "remote-only"} {
		w = callService(s, "GET", "/api/session-search?q="+query, nil)
		var v struct {
			Sessions []struct{ ID, Provider string }
		}
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v.Sessions) != 1 {
			t.Fatal(w.Body.String(), err)
		}
		w = callService(s, "GET", "/api/session-transcript?id="+v.Sessions[0].ID, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "remote-only body") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, method := range methods {
		if method != "GET" {
			t.Fatal("mutation during read-only search", method)
		}
	}
	if len(p.clients) != 0 || len(s.sessions) != 0 || len(s.conversations) != 0 {
		t.Fatal("search created runtime owner")
	}
}

func TestSessionSearchConfiguredClaudeAccountProvenance(t *testing.T) {
	s, _ := testServer(t)
	configDir := t.TempDir()
	root := filepath.Join(configDir, "projects", "project")
	os.MkdirAll(root, 0700)
	os.WriteFile(filepath.Join(root, testID+".jsonl"), []byte(`{"type":"user","cwd":"/project","sessionId":"`+testID+`","message":{"content":"account-scoped needle"}}
{"type":"custom-title","customTitle":"account-scoped title"}
`), 0600)
	s.providers["Claude"] = ClaudeProvider{ConfigDir: configDir, Account: "configured@example.com"}
	s.cfg.Check = func(_ context.Context, resource, action string) error {
		if strings.HasPrefix(resource, "agent/sessions/transcript:claude:") && resource != "agent/sessions/transcript:claude:configured@example.com" {
			return errors.New("wrong account")
		}
		return nil
	}
	w := callService(s, "GET", "/api/session-search?q=needle", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "account-scoped title") {
		t.Fatal(w.Code, w.Body.String())
	}
	var value struct{ Sessions []struct{ ID string } }
	json.Unmarshal(w.Body.Bytes(), &value)
	if len(value.Sessions) != 1 {
		t.Fatal(w.Body.String())
	}
	w = callService(s, "GET", "/api/session-transcript?id="+value.Sessions[0].ID, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "configured@example.com") {
		t.Fatal(w.Code, w.Body.String())
	}
	s.cfg.Check = func(_ context.Context, resource, action string) error {
		if resource == "agent/sessions/transcript:claude:configured@example.com" {
			return errors.New("denied")
		}
		return nil
	}
	w = callService(s, "GET", "/api/session-search?q=needle", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "account-scoped title") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSessionSearchExistingServicePolicyAndExplicitDenies(t *testing.T) {
	for _, denial := range []string{"", "source", "project", "session"} {
		t.Run(denial, func(t *testing.T) {
			s, _ := testServer(t)
			root := filepath.Join(s.cfg.Home, ".codex", "sessions")
			os.MkdirAll(root, 0700)
			os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(`{"type":"session_meta","payload":{"id":"account-test","cwd":"/project"}}
{"type":"response_item","payload":{"type":"message","content":[{"type":"output_text","text":"policy needle"}]}}
`), 0600)
			policyPath := filepath.Join(t.TempDir(), "policy.csv")
			rows := "p, iugum-chat, agent/sessions*, *, allow\n"
			if denial == "source" {
				rows += "p, iugum-chat, agent/sessions/transcript:codex:, *, deny\n"
			}
			if denial == "session" {
				rows += "p, iugum-chat, agent/sessions/Codex/account-test, read, deny\n"
			}
			if denial == "project" {
				rows += "p, iugum-chat, agent/sessions/project:/project, read, deny\n"
			}
			os.WriteFile(policyPath, []byte(rows), 0600)
			gate, err := policy.New("", policyPath)
			if err != nil {
				t.Fatal(err)
			}
			s.cfg.Check = func(ctx context.Context, resource, action string) error {
				return gate.Enforce(ctx, contract.Request{Sub: "iugum-chat", Obj: resource, Act: action})
			}
			w := callService(s, "GET", "/api/session-search?q=needle", nil)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var value struct{ Sessions []struct{ ID string } }
			json.Unmarshal(w.Body.Bytes(), &value)
			if denial == "" {
				if len(value.Sessions) != 1 {
					t.Fatal("existing service grant failed", w.Body.String())
				}
				w = callService(s, "GET", "/api/session-transcript?id="+value.Sessions[0].ID, nil)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			if denial != "" && len(value.Sessions) != 0 {
				t.Fatal("explicit deny leaked results", w.Body.String())
			}
			if denial == "source" {
				index, err := agentsessions.Open(filepath.Join(s.cfg.DataDir, "transcript-search.sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				defer index.Close()
				hits, err := index.Sessions(context.Background(), "", func(agentsessions.Hit) error { return nil })
				if err != nil || len(hits) != 0 {
					t.Fatal("denied source ingested", hits, err)
				}
			}
		})
	}
}

func TestSessionSearchRechecksExplicitDeniesOnTranscriptRead(t *testing.T) {
	for _, resource := range []string{"agent/sessions/transcript:codex:", "agent/sessions/project:/project", "agent/sessions/Codex/read-test"} {
		t.Run(resource, func(t *testing.T) {
			s, _ := testServer(t)
			root := filepath.Join(s.cfg.Home, ".codex", "sessions")
			os.MkdirAll(root, 0700)
			os.WriteFile(filepath.Join(root, "session.jsonl"), []byte(`{"type":"session_meta","payload":{"id":"read-test","cwd":"/project"}}
{"type":"response_item","payload":{"type":"message","content":[{"type":"output_text","text":"private transcript body"}]}}
`), 0600)
			policyPath := filepath.Join(t.TempDir(), "policy.csv")
			base := "p, iugum-chat, agent/sessions*, *, allow\n"
			os.WriteFile(policyPath, []byte(base), 0600)
			gate, err := policy.New("", policyPath)
			if err != nil {
				t.Fatal(err)
			}
			s.cfg.Check = func(ctx context.Context, obj, act string) error {
				return gate.Enforce(ctx, contract.Request{Sub: "iugum-chat", Obj: obj, Act: act})
			}
			w := callService(s, "GET", "/api/session-search?q=private", nil)
			var result struct{ Sessions []struct{ ID string } }
			json.Unmarshal(w.Body.Bytes(), &result)
			if w.Code != 200 || len(result.Sessions) != 1 {
				t.Fatal(w.Code, w.Body.String())
			}
			os.WriteFile(policyPath, []byte(base+"p, iugum-chat, "+resource+", read, deny\n"), 0600)
			gate, err = policy.New("", policyPath)
			if err != nil {
				t.Fatal(err)
			}
			w = callService(s, "GET", "/api/session-transcript?id="+result.Sessions[0].ID, nil)
			if w.Code == 200 || strings.Contains(w.Body.String(), "private transcript body") {
				t.Fatal("current deny bypassed", w.Code, w.Body.String())
			}
		})
	}
}
