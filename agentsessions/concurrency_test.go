package agentsessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedSearchDoesNotWaitForWriterTransaction(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "session.jsonl")
	os.WriteFile(source, []byte(`{"message":{"content":"cached evidence"}}`+"\n"), 0600)
	index, e := Open(filepath.Join(d, "index.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer index.Close()
	if _, e = index.Sync(context.Background(), []Source{{"claude", d, ""}}, Limits{}); e != nil {
		t.Fatal(e)
	}
	// Hold the exact write transaction shape Sync uses while querying from another connection.
	tx, e := index.db.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM transcript_fts`); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	hits, e := index.Search(ctx, Query{Text: "cached"})
	if e != nil || len(hits) != 1 {
		t.Fatalf("cached search blocked or lost snapshot: %v %v", hits, e)
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Fatal("cached read delayed by writer")
	}
	evidence, e := index.Get(ctx, hits[0], EvidenceOptions{Authorize: func(Hit) error { return nil }})
	if e != nil || evidence.Match.Text != "cached evidence" {
		t.Fatal(evidence, e)
	}
}
