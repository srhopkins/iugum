package sqlitemem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestAttachDoesNotCopy(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{NS: "agent-a", Key: "rule", Value: "use JWT"})
	if err := s.Attach(ctx, "agent-a", "agent-b", "rule"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Recall(ctx, "agent-b", "rule")
	if err != nil || !ok || got.Value != "use JWT" {
		t.Fatalf("attach recall: %+v ok=%v err=%v", got, ok, err)
	}
	_ = s.Forget(ctx, "agent-a", "rule")
	_, still, err := s.Recall(ctx, "agent-b", "rule")
	if err != nil || !still {
		t.Fatal("detach of own should not drop last attach until last binding gone")
	}
}

func TestSliceAndJoin(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{NS: "src", Key: "jwt", Value: "use JWT on API"})
	_ = s.Remember(ctx, contract.MemoryRec{NS: "src", Key: "fan", Value: "fan curve 40"})
	if err := s.Slice(ctx, "src", "jwt-only", "jwt"); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, contract.MemoryQuery{NS: []string{"jwt-only"}, Text: "JWT"})
	if err != nil || len(hits) != 1 || hits[0].Key != "jwt" {
		t.Fatalf("slice hits=%+v err=%v", hits, err)
	}
	hits, err = s.Search(ctx, contract.MemoryQuery{NS: []string{"src", "jwt-only"}, Text: "fan"})
	if err != nil || len(hits) == 0 {
		t.Fatalf("join err=%v hits=%+v", err, hits)
	}
}

func TestFTSFindsToken(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{Key: "a", Value: "always run tests with race detector"})
	_ = s.Remember(ctx, contract.MemoryRec{Key: "b", Value: "ship on Friday"})
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "race detector"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Key != "a" {
		t.Fatalf("fts hits=%+v", hits)
	}
}

func TestIngestAndWalk(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	edges, err := s.Ingest(ctx, "lab", "Steve owns the MI50. The MI50 is in the tower.")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges %+v", edges)
	}
	walk, err := s.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk) == 0 {
		t.Fatal("empty walk")
	}
	found := false
	for _, h := range walk {
		if h.Rel == "owns" && h.To == "mi50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("walk %+v", walk)
	}
}

func TestLinkRejectsUnknownRel(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Link(ctx, "lab", contract.MemoryEdge{From: "steve", Rel: "invented", To: "mi50"}); err != nil {
		t.Fatal(err)
	}
	walk, err := s.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve"})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk) != 0 {
		t.Fatalf("unknown rel stored: %+v", walk)
	}
}
