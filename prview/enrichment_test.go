package prview

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSummaryBatchPartialAndCounts(t *testing.T) {
	calls := 0
	s := fakeService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/graphql" {
			t.Error("unexpected request")
		}
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if !strings.Contains(b["query"], "p1:repository") || strings.Contains(b["query"], "test-secret") {
			t.Error("invalid fixed query")
		}
		fmt.Fprint(w, `{"data":{"p0":{"pullRequest":{"reviewDecision":"CHANGES_REQUESTED","comments":{"totalCount":5},"assignees":{"nodes":[{"login":"sam"}]},"stack":{"size":3},"stackEntry":{"position":2},"statusCheckRollup":{"contexts":{"totalCount":9,"checkRunCountsByState":[{"state":"SUCCESS","count":2},{"state":"SKIPPED","count":1},{"state":"FAILURE","count":2},{"state":"IN_PROGRESS","count":1}],"statusContextCountsByState":[{"state":"SUCCESS","count":1},{"state":"ERROR","count":1},{"state":"PENDING","count":1}]}}}},"p1":null},"errors":[{"message":"test-secret sensitive remote error","path":["p1"]}]}`)
	})
	result, e := s.Summaries(context.Background(), url.Values{"pr": {"a/b/1", "a/c/2"}})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "test-secret") {
		t.Fatal("secret leaked")
	}
	var d struct{ Items []Summary }
	json.Unmarshal(b, &d)
	if calls != 1 || len(d.Items) != 2 || d.Items[0].Checks != (CheckCounts{9, 3, 3, 2, 1}) || d.Items[0].Stack.Position != 2 || d.Items[1].Error == "" {
		t.Fatal(string(b))
	}
}
func TestEnrichmentInputs(t *testing.T) {
	s := fakeService(t, func(http.ResponseWriter, *http.Request) { t.Fatal("invalid input reached GitHub") })
	for _, v := range []url.Values{{}, {"pr": make([]string, 31)}, {"pr": {"a/b/1", "a/b/1"}}, {"pr": {"a/../1"}}, {"pr": {"a/b/2147483648"}}, {"pr": {"a/b/1"}, "query": {"mutation{}"}}} {
		if _, e := s.Summaries(context.Background(), v); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	for _, v := range []url.Values{{"cursor": {"abc"}}, {"head": {"bad"}}, {"page": {"0"}}, {"cursor": {"a", "b"}}} {
		if _, e := s.Section(context.Background(), "a", "b", "1", "checks", v); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}
func TestDetailPaginationAndHead(t *testing.T) {
	s := fakeService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if r.URL.Path != "/repos/a/b/pulls/1/comments" || r.URL.Query().Get("page") != "2" {
				t.Error(r.URL)
			}
			w.Header().Set("Link", `<https://api.github.com/next>; rel="next"`)
			fmt.Fprint(w, `[{"id":42,"body":"hello","html_url":"https://github.com/a/b/pull/1#x","created_at":"now","path":"a.go","user":{"login":"helper[bot]","type":"Bot"}}]`)
			return
		}
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"headRefOid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","statusCheckRollup":{"contexts":{"nodes":[{"__typename":"CheckRun","id":"c1","name":"build","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://github.com/check","checkSuite":{"app":{"slug":"actions"}}}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}}}`)
	})
	p, e := s.Section(context.Background(), "a", "b", "1", "code-comments", url.Values{"page": {"2"}})
	if e != nil || len(p.Items) != 1 || !p.Items[0].Bot || p.Items[0].Path != "a.go" || p.Items[0].CreatedAt != "now" || !p.HasNext {
		t.Fatal(p, e)
	}
	p, e = s.Section(context.Background(), "a", "b", "1", "checks", nil)
	if e != nil || p.NextCursor != "next" || !p.HasNext || p.Items[0].State != "SUCCESS" {
		t.Fatal(p, e)
	}
	_, e = s.Section(context.Background(), "a", "b", "1", "checks", url.Values{"cursor": {"next"}, "head": {strings.Repeat("b", 40)}})
	if api, ok := e.(*APIError); !ok || api.Code != "head_changed" {
		t.Fatal(e)
	}
}
func TestGraphErrorsSanitized(t *testing.T) {
	for _, body := range []string{`{"data":null,"errors":[{"message":"test-secret"}]}`, `{"data":{},"errors":[{"message":"test-secret"}]}`} {
		s := fakeService(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		_, e := s.Summaries(context.Background(), url.Values{"pr": {"a/b/1"}})
		if e == nil || strings.Contains(e.Error(), "test-secret") {
			t.Fatal(e)
		}
	}
}
