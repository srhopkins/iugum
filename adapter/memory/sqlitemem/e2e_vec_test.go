//go:build linux || darwin || freebsd || netbsd || openbsd || windows

package sqlitemem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

// E2E-style test: vec0 KNN ranks cat over dog with fake embedder.
func TestE2EVecSearchCatOverDog(t *testing.T) {
	ctx := context.Background()
	cat := []float32{1, 0, 0, 0}
	dog := []float32{0, 1, 0, 0}
	emb := fakeEmbed{m: map[string][]float32{
		"the cat sat":    cat,
		"the dog barked": dog,
		"feline":         cat,
	}}
	s, err := OpenOpts(filepath.Join(t.TempDir(), "e2e.db"), Opts{Embedder: emb, Vec: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.vec.probe(s.db); err != nil {
		t.Skip("sqlite-vec init failed on this platform: ", err)
	}
	if !s.vec.ready {
		t.Skip("sqlite-vec index not active on this platform")
	}
	if err := s.Remember(ctx, contract.MemoryRec{Key: "cat", Value: "the cat sat"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remember(ctx, contract.MemoryRec{Key: "dog", Value: "the dog barked"}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "feline", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 1 || hits[0].Key != "cat" {
		t.Fatalf("want cat first, got %+v", hits)
	}
}
