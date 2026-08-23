package sqlitemem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestSlugMergeTarget(t *testing.T) {
	existing := []string{"steve", "mi50", "steve-hopkins", "tower"}
	if got := slugMergeTarget("steve-hopkins", existing); got != "steve" {
		t.Fatalf("steve-hopkins → %q, want steve", got)
	}
	if got := slugMergeTarget("steve", existing); got != "" {
		t.Fatalf("steve → %q, want empty", got)
	}
	if got := slugMergeTarget("mi50", existing); got != "" {
		t.Fatalf("mi50 → %q, want empty", got)
	}
}

func TestMergeGraphNodesSlugAlias(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Link(ctx, "lab", contract.MemoryEdge{From: "steve", Rel: "owns", To: "mi50"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(ctx, "lab", contract.MemoryEdge{From: "steve-hopkins", Rel: "owns", To: "tower"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err = s.mergeGraphNodesLocked(ctx, "lab")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	keys, err := s.graphNodeKeys(ctx, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("graph keys after merge: %v", keys)
	}
	for _, k := range keys {
		if k == "steve-hopkins" {
			t.Fatal("steve-hopkins should merge into steve")
		}
	}

	walk, err := s.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 1})
	if err != nil {
		t.Fatal(err)
	}
	owns := map[string]bool{}
	for _, h := range walk {
		if h.Rel == "owns" {
			owns[h.To] = true
		}
	}
	if !owns["mi50"] || !owns["tower"] {
		t.Fatalf("steve walk should include mi50 and tower, got %+v", walk)
	}
}

func TestMergeGraphNodesCosine(t *testing.T) {
	ctx := context.Background()
	steve := []float32{1, 0, 0, 0}
	stephen := []float32{0.99, 0.01, 0, 0}
	emb := fakeEmbed{m: map[string][]float32{
		"steve":  steve,
		"stephen": stephen,
	}}
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), emb)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Link(ctx, "lab", contract.MemoryEdge{From: "steve", Rel: "owns", To: "mi50"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Link(ctx, "lab", contract.MemoryEdge{From: "stephen", Rel: "uses", To: "tower"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	err = s.mergeGraphNodesLocked(ctx, "lab")
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	keys, err := s.graphNodeKeys(ctx, "lab")
	if err != nil {
		t.Fatal(err)
	}
	personKeys := 0
	for _, k := range keys {
		if k == "steve" || k == "stephen" {
			personKeys++
		}
	}
	if personKeys != 1 {
		t.Fatalf("want one merged person node, keys=%v", keys)
	}
}

func TestIngestMergesDuplicatePerson(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.Ingest(ctx, "lab", "Steve owns the MI50."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(ctx, "lab", "Steve Hopkins owns the MI50."); err != nil {
		t.Fatal(err)
	}

	keys, err := s.graphNodeKeys(ctx, "lab")
	if err != nil {
		t.Fatal(err)
	}
	personLike := 0
	for _, k := range keys {
		if k == "steve" || k == "steve-hopkins" {
			personLike++
		}
	}
	if personLike != 1 {
		t.Fatalf("want one person node, keys=%v", keys)
	}

	walk, err := s.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 1, Rel: "owns"})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk) != 1 || walk[0].To != "mi50" {
		t.Fatalf("want single owns→mi50 from steve, got %+v", walk)
	}
}
