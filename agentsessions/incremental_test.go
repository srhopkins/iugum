package agentsessions

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncrementalUpdateDelete(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s.jsonl")
	write := func(txt string) {
		t.Helper()
		if e := os.WriteFile(p, []byte(`{"message":{"content":"`+txt+`"}}`+"\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("first")
	i, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	sources := []Source{{"claude", d, ""}}
	r, e := i.Sync(context.Background(), sources, Limits{})
	if e != nil || r.TotalMessages != 1 {
		t.Fatal(r, e)
	}
	r, e = i.Sync(context.Background(), sources, Limits{})
	if e != nil || r.Messages != 0 || r.Unchanged != 1 {
		t.Fatal(r, e)
	}
	write("second replacement")
	r, e = i.Sync(context.Background(), sources, Limits{})
	if e != nil || r.Messages != 1 || r.TotalMessages != 1 {
		t.Fatal(r, e)
	}
	h, _ := i.Search(context.Background(), Query{Text: "first"})
	if len(h) != 0 {
		t.Fatal(h)
	}
	os.Remove(p)
	r, e = i.Sync(context.Background(), sources, Limits{})
	if e != nil || r.TotalMessages != 0 {
		t.Fatal(r, e)
	}
}
func TestLargeJSONLAndOversizedLine(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s.jsonl")
	text := strings.Repeat(`{"message":{"content":"small"}}`+"\n", 50) + strings.Repeat("x", 512) + "\n" + `{"message":{"content":"after oversized"}}` + "\n"
	os.WriteFile(p, []byte(text), 0600)
	i, _ := Open(":memory:")
	defer i.Close()
	r, e := i.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{MaxFileBytes: 128})
	if e != nil || r.TotalMessages != 51 || !r.Truncated {
		t.Fatal(r, e)
	}
	h, _ := i.Search(context.Background(), Query{Text: "after"})
	if len(h) != 1 || h[0].Line != 52 {
		t.Fatal(h)
	}
}
func TestCursorCanonicalOrphanAndUpdate(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state.vscdb")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`CREATE TABLE cursorDiskKV(key TEXT PRIMARY KEY,value BLOB); CREATE TABLE composerHeaders(composerId TEXT,workspaceId TEXT); INSERT INTO composerHeaders VALUES('session','workspace'); INSERT INTO cursorDiskKV VALUES('composerData:session','{"fullConversationHeadersOnly":[]}'); INSERT INTO cursorDiskKV VALUES('bubbleId:session:orphan','{"type":2,"createdAt":"2026-09-10T12:00:00Z","thinking":{"text":"reasoning evidence"},"toolFormerData":{"name":"read","params":{"path":"file"},"result":{"text":"tool evidence"}}}');`)
	if e != nil {
		t.Fatal(e)
	}
	i, _ := Open(":memory:")
	defer i.Close()
	s := []Source{{"cursor", p, "personal"}}
	r, e := i.Sync(context.Background(), s, Limits{})
	if e != nil || r.TotalMessages != 1 || len(r.Warnings) != 0 {
		t.Fatal(r, e)
	}
	h, e := i.Search(context.Background(), Query{Text: "reasoning evidence"})
	if e != nil || len(h) != 1 || !strings.Contains(h[0].Path, "orphan") || h[0].Timestamp != "2026-09-10T12:00:00Z" {
		t.Fatal(h, e)
	}
	_, e = db.Exec(`DELETE FROM cursorDiskKV WHERE key LIKE 'bubbleId:%'`)
	if e != nil {
		t.Fatal(e)
	}
	r, e = i.Sync(context.Background(), s, Limits{})
	if e != nil || r.TotalMessages != 0 {
		t.Fatal(r, e)
	}
}

func TestRemovedSourceAndCancelledRefresh(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "s.jsonl")
	os.WriteFile(p, []byte(`{"message":{"content":"retained"}}`+"\n"), 0600)
	i, _ := Open(":memory:")
	defer i.Close()
	sources := []Source{{"claude", d, ""}}
	i.Sync(context.Background(), sources, Limits{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := i.Sync(ctx, nil, Limits{}); e == nil {
		t.Fatal("expected cancellation")
	}
	hits, _ := i.Search(context.Background(), Query{Text: "retained"})
	if len(hits) != 1 {
		t.Fatal("cancelled refresh lost records")
	}
	r, e := i.Sync(context.Background(), nil, Limits{})
	if e != nil || r.TotalMessages != 0 {
		t.Fatal(r, e)
	}
}
func TestCursorWALAndWorkspace(t *testing.T) {
	d := t.TempDir()
	global := filepath.Join(d, "User", "globalStorage")
	workspace := filepath.Join(d, "User", "workspaceStorage", "ws")
	os.MkdirAll(global, 0700)
	os.MkdirAll(workspace, 0700)
	os.WriteFile(filepath.Join(workspace, "workspace.json"), []byte(`{"folder":"file:///work/ffai"}`), 0600)
	p := filepath.Join(global, "state.vscdb")
	db, _ := sql.Open("sqlite", p)
	defer db.Close()
	_, e := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE composerHeaders(composerId TEXT,workspaceId TEXT);CREATE TABLE cursorDiskKV(key TEXT,value BLOB);INSERT INTO composerHeaders VALUES('s','ws');INSERT INTO cursorDiskKV VALUES('bubbleId:s:b','{"text":"initial"}');`)
	if e != nil {
		t.Fatal(e)
	}
	i, _ := Open(":memory:")
	defer i.Close()
	sources := []Source{{"cursor", p, ""}}
	if _, e = i.Sync(context.Background(), sources, Limits{}); e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE cursorDiskKV SET value='{"text":"updated"}'`)
	r, e := i.Sync(context.Background(), sources, Limits{})
	if e != nil || r.Messages != 1 {
		t.Fatal(r, e)
	}
	hits, _ := i.Search(context.Background(), Query{Text: "updated", Scope: "ffai"})
	if len(hits) != 1 || hits[0].Project != "/work/ffai" {
		t.Fatal(hits)
	}
}

func TestBoundedBatchesEventuallyComplete(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "large.jsonl")
	os.WriteFile(p, []byte(strings.Repeat(`{"message":{"content":"evidence"}}`+"\n", 7)), 0600)
	i, _ := Open(":memory:")
	defer i.Close()
	sources := []Source{{"claude", d, ""}}
	for _, total := range []int{2, 4, 6, 7} {
		r, e := i.Sync(context.Background(), sources, Limits{MaxMessages: 2})
		if e != nil || r.TotalMessages != total || r.Messages > 2 {
			t.Fatal(total, r, e)
		}
	}
	r, e := i.Sync(context.Background(), sources, Limits{MaxMessages: 2})
	if e != nil || r.Unchanged != 1 || r.Truncated {
		t.Fatal(r, e)
	}
}

func TestCursorBoundedBatches(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.vscdb")
	db, _ := sql.Open("sqlite", p)
	defer db.Close()
	db.Exec(`CREATE TABLE cursorDiskKV(key TEXT,value TEXT)`)
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		db.Exec(`INSERT INTO cursorDiskKV VALUES(?,?)`, "bubbleId:s:"+key, `{"text":"bubble"}`)
	}
	i, _ := Open(":memory:")
	defer i.Close()
	for _, want := range []int{2, 4, 5} {
		r, e := i.Sync(context.Background(), []Source{{"cursor", p, ""}}, Limits{MaxMessages: 2})
		if e != nil || r.TotalMessages != want {
			t.Fatal(want, r, e)
		}
	}
}
