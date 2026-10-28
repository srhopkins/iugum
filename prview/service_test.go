package prview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFilters(t *testing.T) {
	f, err := Parse(url.Values{"org": {"alpha", "beta"}, "repo": {"alpha/api", "beta/web"}, "author": {"sam", "jo"}, "q": {"fix OR bug"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `is:pr state:open (org:alpha OR org:beta) (repo:alpha/api OR repo:beta/web) (author:sam OR author:jo) "fix OR bug"`
	if f.Query() != want {
		t.Fatalf("query %q", f.Query())
	}
	for _, v := range []url.Values{{"q": {"is:issue"}}, {"state": {"merged"}}, {"page": {"0"}}, {"per_page": {"101"}}, {"author": {"sam OR is:issue"}}, {"org": {"a", "b", "c", "d", "e", "f", "g"}}, {"sort": {"title"}}, {"unknown": {"x"}}, {"state": {"open", "closed"}}, {"repo": {"a/.."}}} {
		if _, err := Parse(v); err == nil {
			t.Fatalf("accepted invalid filters %v", v)
		}
	}
	f, err = Parse(url.Values{"org": {"Alpha", "alpha"}, "state": {"all"}, "draft": {"no"}})
	if err != nil || len(f.Orgs) != 1 || f.Query() != "is:pr draft:false org:Alpha" {
		t.Fatal(f, err)
	}
	if _, err = Parse(url.Values{"page": {"34"}, "per_page": {"30"}}); err != nil {
		t.Fatal("last partial page rejected", err)
	}
	if _, err = Parse(url.Values{"page": {"35"}, "per_page": {"30"}}); err == nil {
		t.Fatal("past ceiling accepted")
	}
}
func fakeService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Service{client: server.Client(), base: server.URL, resolve: func(context.Context) (string, error) { return "test-secret", nil }}
}
func TestSearchAndDetail(t *testing.T) {
	var base string
	s := fakeService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("missing authentication/version")
		}
		if r.URL.Path == "/search/issues" {
			if r.URL.Query().Get("advanced_search") != "true" {
				t.Error("advanced search required for boolean semantics")
			}
			if r.URL.Query().Get("q") != "is:pr state:open author:sam" {
				t.Error(r.URL.Query())
			}
			json.NewEncoder(w).Encode(map[string]any{"total_count": 1001, "incomplete_results": true, "items": []any{map[string]any{"number": 7, "title": "Fix", "repository_url": base + "/repos/a/b", "html_url": "https://github.com/a/b/pull/7", "user": map[string]string{"login": "sam"}, "state": "open", "draft": true, "pull_request": map[string]string{"url": "detail"}}}})
			return
		}
		if r.URL.Path != "/repos/a/b/pulls/7" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"number":7,"title":"Fix","body":"Details","merged":false,"state":"open","user":{"login":"sam"}}`))
	})
	base = s.base
	f, _ := Parse(url.Values{"author": {"sam"}})
	result, err := s.Search(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Repository != "a/b" || !result.Items[0].Draft || result.Items[0].Review != "not_loaded" || !result.Truncated || !result.Incomplete || result.FetchedAt.IsZero() {
		t.Fatal(result)
	}
	detail, err := s.Detail(context.Background(), "a", "b", "7")
	if err != nil || detail.Body != "Details" || detail.Merged == nil || *detail.Merged {
		t.Fatal(detail, err)
	}
}
func TestErrorsNeverExposeUpstreamBody(t *testing.T) {
	for _, code := range []int{401, 403, 404, 422, 429, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			s := fakeService(t, func(w http.ResponseWriter, r *http.Request) {
				if code == 429 {
					w.Header().Set("Retry-After", "60")
				}
				w.WriteHeader(code)
				w.Write([]byte(`{"message":"test-secret private payload"}`))
			})
			handler := Handler(http.NotFoundHandler(), s)
			r := httptest.NewRequest("GET", "http://localhost/api/v1/prs", nil)
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if strings.Contains(w.Body.String(), "test-secret") || strings.Contains(w.Body.String(), "private payload") || w.Code < 400 {
				t.Fatal(w.Code, w.Body.String())
			}
			if code == 429 && (w.Code != 429 || w.Header().Get("Retry-After") != "60") {
				t.Fatal(w)
			}
		})
	}
}
func TestHTTPBoundaries(t *testing.T) {
	s := fakeService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream call") })
	h := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), s)
	for _, tc := range []struct {
		path, method, host, remote, origin string
		status                             int
	}{
		{"/api/beads", "GET", "localhost", "10.0.0.1:5", "", 204},
		{"/api/v1/meta", "GET", "localhost", "127.0.0.1:5", "", 200},
		{"/api/v1/health", "GET", "localhost", "127.0.0.1:5", "", 200},
		{"/api/v1/prs", "POST", "localhost", "127.0.0.1:5", "", 405},
		{"/api/v1/prs", "GET", "attacker.example", "127.0.0.1:5", "", 403},
		{"/api/v1/prs", "GET", "localhost", "192.168.1.2:5", "", 403},
		{"/api/v1/prs", "GET", "localhost", "127.0.0.1:5", "http://evil.example", 403},
		{"/api/v1/prs?state=nope", "GET", "localhost", "127.0.0.1:5", "", 400},
		{"/api/v1/nope", "GET", "localhost", "127.0.0.1:5", "", 404},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
}
func TestCredentialPrecedence(t *testing.T) {
	t.Setenv("GH_TOKEN", "first")
	t.Setenv("GITHUB_TOKEN", "second")
	token, err := credentials(context.Background())
	if token != "first" || err != nil {
		t.Fatal("precedence")
	}
	t.Setenv("GH_TOKEN", "")
	token, err = credentials(context.Background())
	if token != "second" || err != nil {
		t.Fatal("fallback")
	}
}
func TestRejectNonPR(t *testing.T) {
	s := fakeService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total_count":1,"items":[{"number":1}]}`))
	})
	f, _ := Parse(nil)
	if _, err := s.Search(context.Background(), f); err == nil {
		t.Fatal("accepted issue")
	}
}

func TestBotAuthor(t *testing.T) {
	f, err := Parse(url.Values{"author": {"dependabot[bot]"}})
	if err != nil || f.Query() != "is:pr state:open author:dependabot[bot]" {
		t.Fatal(f, err)
	}
}
