package sqlitemem

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

// fakeEmbed maps words to one-hot-ish vectors so "cat" is near "kitten" only if we teach it.
type fakeEmbed struct {
	m map[string][]float32
}

func (f fakeEmbed) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := f.m[t]; ok {
			out[i] = v
			continue
		}
		// default: hash first letter
		v := make([]float32, 4)
		if t != "" {
			v[int(t[0])%4] = 1
		}
		out[i] = v
	}
	return out, nil
}

func TestRememberRecallForget(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Remember(ctx, contract.MemoryRec{Key: "auth-jwt", Value: "use JWT"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Recall(ctx, "", "auth-jwt")
	if err != nil || !ok || got.Value != "use JWT" {
		t.Fatalf("recall: %+v ok=%v err=%v", got, ok, err)
	}
	if err := s.Forget(ctx, "", "auth-jwt"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = s.Recall(ctx, "", "auth-jwt")
	if err != nil || ok {
		t.Fatalf("forgot still present ok=%v err=%v", ok, err)
	}
}

func TestSearchSubstringWhenEmbedOff(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{Key: "a", Value: "always run tests with race"})
	_ = s.Remember(ctx, contract.MemoryRec{Key: "b", Value: "ship on Friday"})
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "race"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Key != "a" {
		t.Fatalf("hits=%+v", hits)
	}
}

func TestSearchByEmbedding(t *testing.T) {
	ctx := context.Background()
	cat := []float32{1, 0, 0, 0}
	dog := []float32{0, 1, 0, 0}
	emb := fakeEmbed{m: map[string][]float32{
		"the cat sat":     cat,
		"a feline rest":   cat,
		"the dog barked":  dog,
		"feline":          cat,
	}}
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), emb)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{Key: "cat", Value: "the cat sat"})
	_ = s.Remember(ctx, contract.MemoryRec{Key: "dog", Value: "the dog barked"})
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "feline", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Key != "cat" {
		t.Fatalf("want cat first, got %+v", hits)
	}
}

func TestDisabledEmbeddingsNoVectorNeeded(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "m.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Remember(ctx, contract.MemoryRec{Key: "x", Value: "plain text"})
	hits, err := s.Search(ctx, contract.MemoryQuery{Text: "plain"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("err=%v hits=%+v", err, hits)
	}
}
