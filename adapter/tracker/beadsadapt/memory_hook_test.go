package beadsadapt

import (
	"context"
	"strings"
	"testing"

	bdcmd "github.com/steveyegge/beads/cmd/bd"

	"github.com/srhopkins/iugum/contract"
)

type fakeMemory struct {
	data map[string]string
}

func (f *fakeMemory) Name() string { return "fake" }

func (f *fakeMemory) Remember(_ context.Context, rec contract.MemoryRec) error {
	if f.data == nil {
		f.data = map[string]string{}
	}
	f.data[rec.Key] = rec.Value
	return nil
}

func (f *fakeMemory) Recall(_ context.Context, _, key string) (contract.MemoryRec, bool, error) {
	v, ok := f.data[key]
	if !ok {
		return contract.MemoryRec{}, false, nil
	}
	return contract.MemoryRec{NS: beadsMemNS, Type: beadsMemType, Key: key, Value: v}, true, nil
}

func (f *fakeMemory) Forget(_ context.Context, _, key string) error {
	delete(f.data, key)
	return nil
}

func (f *fakeMemory) Search(_ context.Context, q contract.MemoryQuery) ([]contract.MemoryHit, error) {
	needle := strings.ToLower(strings.TrimSpace(q.Text))
	var hits []contract.MemoryHit
	for k, v := range f.data {
		if needle != "" && !strings.Contains(strings.ToLower(k), needle) && !strings.Contains(strings.ToLower(v), needle) {
			continue
		}
		hits = append(hits, contract.MemoryHit{NS: beadsMemNS, Type: beadsMemType, Key: k, Value: v, Score: 1})
	}
	return hits, nil
}

func (f *fakeMemory) Attach(context.Context, string, string, string) error   { return nil }
func (f *fakeMemory) Detach(context.Context, string, string) error             { return nil }
func (f *fakeMemory) Slice(context.Context, string, string, string) error      { return nil }
func (f *fakeMemory) Link(context.Context, string, contract.MemoryEdge) error  { return nil }
func (f *fakeMemory) Unlink(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeMemory) Walk(context.Context, contract.WalkQuery) ([]contract.WalkHit, error) {
	return nil, nil
}
func (f *fakeMemory) Ingest(context.Context, string, string) ([]contract.MemoryEdge, error) {
	return nil, nil
}

func TestUseMemoryHook(t *testing.T) {
	fm := &fakeMemory{data: map[string]string{}}
	UseMemory(fm)
	bdcmd.SetMemoryHook(sqliteHook)

	h := sqliteHook
	if err := h.Remember("race-flag", "always run tests with -race"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := h.Recall("race-flag")
	if err != nil || !ok || got != "always run tests with -race" {
		t.Fatalf("Recall: %q ok=%v err=%v", got, ok, err)
	}
	all, err := h.List("")
	if err != nil || len(all) != 1 || all["race-flag"] != "always run tests with -race" {
		t.Fatalf("List all: %v err=%v", all, err)
	}
	filtered, err := h.List("race")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("List race: %v err=%v", filtered, err)
	}
	if err := h.Forget("race-flag"); err != nil {
		t.Fatal(err)
	}
	_, ok, err = h.Recall("race-flag")
	if err != nil || ok {
		t.Fatalf("after forget: ok=%v err=%v", ok, err)
	}
	if len(fm.data) != 0 {
		t.Fatalf("fake memory not cleared: %v", fm.data)
	}
}
