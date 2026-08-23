//go:build linux || darwin || freebsd || netbsd || openbsd || windows

package sqlitemem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestVecProbe(t *testing.T) {
	s, err := OpenOpts(filepath.Join(t.TempDir(), "m.db"), Opts{Vec: true, Embedder: fakeEmbed{m: map[string][]float32{"x": {1, 0, 0, 0}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.vec.ready {
		t.Skip("sqlite-vec probe failed on this platform")
	}
}

func TestSearchByEmbeddingVec(t *testing.T) {
	ctx := context.Background()
	cat := []float32{1, 0, 0, 0}
	dog := []float32{0, 1, 0, 0}
	emb := fakeEmbed{m: map[string][]float32{
		"the cat sat":    cat,
		"the dog barked": dog,
		"feline":         cat,
	}}
	s, err := OpenOpts(filepath.Join(t.TempDir(), "m.db"), Opts{Embedder: emb, Vec: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.vec.ready {
		t.Skip("sqlite-vec unavailable on this platform")
	}
	_ = s.Remember(ctx, contract.MemoryRec{Key: "cat", Value: "the cat sat"})
	_ = s.Remember(ctx, contract.MemoryRec{Key: "dog", Value: "the dog barked"})
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "feline", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Key != "cat" {
		t.Fatalf("want cat first with vec, got %+v", hits)
	}
}

func TestVecOffUsesCosine(t *testing.T) {
	TestSearchByEmbedding(t)
}

func TestVecDisabledWithoutEmbedder(t *testing.T) {
	s, err := OpenOpts(filepath.Join(t.TempDir(), "m.db"), Opts{Vec: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.vec.active() {
		t.Fatal("vec should stay off without embedder")
	}
}
