package agentsessions

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestIndexScopeAndProvenance(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "session.jsonl")
	os.WriteFile(p, []byte(`{"type":"user","cwd":"/work/ffai","sessionId":"abc","timestamp":"2026-09-10T12:00:00Z","message":{"content":[{"type":"text","text":"Ticket delivery tomorrow"}]}}
{"type":"assistant","message":{"content":"Delivery confirmed"}}
`), 0600)
	i, e := Open(filepath.Join(d, "index.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	r, e := i.Sync(context.Background(), []Source{{"claude", d, "contract"}}, Limits{})
	if e != nil || r.Messages != 2 {
		t.Fatalf("%+v %v", r, e)
	}
	h, e := i.Search(context.Background(), Query{Text: "ticket delivery", Scope: "ffai"})
	if e != nil || len(h) != 1 {
		t.Fatalf("%+v %v", h, e)
	}
	if h[0].Session != "abc" || h[0].Line != 1 || h[0].Account != "contract" {
		t.Fatal(h)
	}
	h, e = i.Search(context.Background(), Query{Text: "delivery", Exclude: []string{"ffai"}})
	if e != nil || len(h) != 0 {
		t.Fatal(h, e)
	}
}
func TestBoundAndCancellation(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "s.jsonl"), []byte("{\"message\":{\"content\":\"one\"}}\n{\"message\":{\"content\":\"two\"}}\n"), 0600)
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	r, e := i.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{MaxMessages: 1})
	if e != nil || !r.Truncated || r.Messages != 1 {
		t.Fatal(r, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = i.Sync(ctx, nil, Limits{}); e == nil {
		t.Fatal("expected cancellation")
	}
}
func TestResumeRequiresExplicitAccount(t *testing.T) {
	_, e := (Claude{}).Resume(context.Background(), ResumeRequest{SessionID: "abc", Prompt: "hello"})
	if e == nil {
		t.Fatal("expected validation")
	}
	if (Claude{}).Steer(context.Background(), ResumeRequest{}) != ErrLiveSteeringUnsupported {
		t.Fatal("unexpected capability")
	}
}

func TestPolicyDoesNotConsumeLimit(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "s.jsonl"), []byte("{\"cwd\":\"private\",\"message\":{\"content\":\"delivery\"}}\n{\"cwd\":\"public\",\"message\":{\"content\":\"delivery later\"}}\n"), 0600)
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	if _, e = i.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{}); e != nil {
		t.Fatal(e)
	}
	h, e := i.Search(context.Background(), Query{Text: "delivery", Limit: 1, Authorize: func(h Hit) error {
		if h.Project == "private" {
			return os.ErrPermission
		}
		return nil
	}})
	if e != nil || len(h) != 1 || h[0].Project != "public" {
		t.Fatal(h, e)
	}
}

func TestOpenCodeTextProvenance(t *testing.T) {
	p := filepath.Join(t.TempDir(), "opencode.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE session(id TEXT,directory TEXT,time_updated INTEGER); CREATE TABLE part(id TEXT,session_id TEXT,time_created INTEGER,data TEXT); INSERT INTO session VALUES('s1','/work/ffai',2000000000000); INSERT INTO part VALUES('p1','s1',1000000000000,'{"type":"text","text":"delivery evidence"}');`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	r, e := i.Sync(context.Background(), []Source{{"opencode", p, "personal"}}, Limits{})
	if e != nil || len(r.Warnings) > 0 {
		t.Fatal(r, e)
	}
	hits, e := i.Search(context.Background(), Query{Text: "evidence"})
	if e != nil || len(hits) != 1 {
		t.Fatal(hits, e)
	}
	if hits[0].Timestamp != "2001-09-09T01:46:40Z" || hits[0].Path != p+"#part=p1" {
		t.Fatal(hits)
	}
}

func TestEvidenceStoredPolicyAndBounds(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s.jsonl")
	os.WriteFile(p, []byte("{\"cwd\":\"private\",\"message\":{\"content\":\"secret\"}}\n{\"cwd\":\"public\",\"message\":{\"content\":\"delivery evidence detail\"}}\n"), 0600)
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	if _, e = i.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{}); e != nil {
		t.Fatal(e)
	}
	hits, e := i.Search(context.Background(), Query{Text: "delivery"})
	if e != nil || len(hits) != 1 {
		t.Fatal(hits, e)
	}
	auth := func(h Hit) error {
		if h.Project == "private" {
			return os.ErrPermission
		}
		return nil
	}
	evidence, e := i.Get(context.Background(), hits[0], EvidenceOptions{Before: 2, MaxBytes: 8, Authorize: auth})
	if e != nil || !evidence.Truncated || len(evidence.Match.Text) != 8 || len(evidence.Context) != 0 {
		t.Fatal(evidence, e)
	}
	fake := hits[0]
	fake.Path = "/etc/passwd"
	if _, e = i.Get(context.Background(), fake, EvidenceOptions{Authorize: auth}); e != ErrNotIndexed {
		t.Fatal(e)
	}
	private := hits[0]
	private.Line = 1
	private.Project = "public"
	if _, e = i.Get(context.Background(), private, EvidenceOptions{Authorize: auth}); e != os.ErrPermission {
		t.Fatal("must authorize stored metadata", e)
	}
	if _, e = i.Get(context.Background(), hits[0], EvidenceOptions{}); e != ErrAuthorizationRequired {
		t.Fatal(e)
	}
}
func TestExplicitRelaxedSearch(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "s.jsonl"), []byte("{\"message\":{\"content\":\"delivery tomorrow\"}}\n"), 0600)
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	i.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{})
	r, e := i.SearchFallback(context.Background(), Query{Text: "delivery unicorn"})
	if e != nil || r.MatchMode != "any_terms" || len(r.Hits) != 1 {
		t.Fatal(r, e)
	}
	r, e = i.SearchFallback(context.Background(), Query{Text: "delivery tomorrow"})
	if e != nil || r.MatchMode != "all_terms" || len(r.Hits) != 1 {
		t.Fatal(r, e)
	}
}
