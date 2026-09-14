package agentclones

import (
	"context"
	"errors"
	"github.com/srhopkins/iugum/adapter/memory/graphgloss"
	"github.com/srhopkins/iugum/adapter/memory/sqlitemem"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotIsolationPolicyGraphDiff(t *testing.T) {
	ctx := context.Background()
	d := t.TempDir()
	source := filepath.Join(d, "source.db")
	target := filepath.Join(d, "clone.db")
	s, e := sqlitemem.OpenOpts(source, sqlitemem.Opts{Glossary: graphgloss.File{Name: "test"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, r := range []contract.MemoryRec{{NS: "chief", Type: "fact", Key: "a", Value: "original"}, {NS: "chief", Type: "fact", Key: "b", Value: "second"}, {NS: "chief", Type: "private", Key: "secret", Value: "hidden"}, {NS: "other", Type: "fact", Key: "unrelated", Value: "excluded"}} {
		if e = s.Remember(ctx, r); e != nil {
			t.Fatal(e)
		}
	}
	s.Link(ctx, "chief", contract.MemoryEdge{From: "a", Rel: "custom", To: "b", Value: "edge"})
	s.Link(ctx, "chief", contract.MemoryEdge{From: "a", Rel: "custom", To: "secret"})
	check := func(_ context.Context, obj, act string) error {
		if obj == contract.MemoryObj("private", "chief") {
			return errors.New("denied")
		}
		return nil
	}
	r, e := SnapshotMemory(ctx, source, target, "chief", "next", check)
	if e != nil || r.Records != 2 || r.Edges != 1 {
		t.Fatal(r, e)
	}
	clone, e := sqlitemem.Open(target, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer clone.Close()
	clone.Remember(ctx, contract.MemoryRec{NS: "next", Type: "fact", Key: "a", Value: "changed"})
	clone.Forget(ctx, "next", "b")
	clone.Remember(ctx, contract.MemoryRec{NS: "next", Type: "fact", Key: "new", Value: "added"})
	original, ok, e := s.Recall(ctx, "chief", "a")
	if e != nil || !ok || original.Value != "original" {
		t.Fatal(original, e)
	}
	diff, e := DiffMemory(ctx, r.ManifestPath, target, check)
	if e != nil || len(diff.Changed) != 1 || len(diff.Added) != 1 || len(diff.Deleted) != 1 || len(diff.DeletedEdges) != 1 {
		t.Fatal(diff, e)
	}
	if _, ok, e = clone.Recall(ctx, "next", "secret"); e != nil || ok {
		t.Fatal("denied memory copied")
	}
	b, _ := os.ReadFile(r.ManifestPath)
	if len(b) == 0 {
		t.Fatal("missing baseline")
	}
}
func TestSnapshotMissingAndExistingTarget(t *testing.T) {
	d := t.TempDir()
	check := func(context.Context, string, string) error { return nil }
	target := filepath.Join(d, "target.db")
	r, e := SnapshotMemory(context.Background(), filepath.Join(d, "absent.db"), target, "chief", "clone", check)
	if e != nil || !r.SourceMissing || r.Records != 0 {
		t.Fatal(r, e)
	}
	if _, e = SnapshotMemory(context.Background(), "absent", target, "chief", "clone", check); e == nil {
		t.Fatal("must not overwrite")
	}
}
