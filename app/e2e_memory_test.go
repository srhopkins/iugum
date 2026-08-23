package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestE2EMemoryRememberRecallSearch(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := a.Remember(ctx, contract.MemoryRec{Key: "jwt-rule", Value: "always use JWT on API routes"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := a.Recall(ctx, "", "jwt-rule")
	if err != nil || !ok || got.Value != "always use JWT on API routes" {
		t.Fatalf("recall: %+v ok=%v err=%v", got, ok, err)
	}

	hits, err := a.SearchMem(ctx, contract.MemoryQuery{Text: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Key != "jwt-rule" {
		t.Fatalf("search hits=%+v", hits)
	}
}

func TestE2EMemoryForgetRemoves(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := a.Remember(ctx, contract.MemoryRec{Key: "temp", Value: "gone soon"}); err != nil {
		t.Fatal(err)
	}
	if err := forgetMem(ctx, a, "", "temp"); err != nil {
		t.Fatal(err)
	}
	_, ok, err := a.Recall(ctx, "", "temp")
	if err != nil || ok {
		t.Fatalf("want missing after forget, ok=%v err=%v", ok, err)
	}
}

func TestE2EMemoryRecallMissing(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	_, ok, err := a.Recall(ctx, "", "no-such-key")
	if err != nil || ok {
		t.Fatalf("want absent key, ok=%v err=%v", ok, err)
	}
}

func TestE2EMemoryAttachSliceVisibility(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := a.Remember(ctx, contract.MemoryRec{NS: "src", Key: "jwt", Value: "use JWT on API"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(ctx, contract.MemoryRec{NS: "src", Key: "fan", Value: "fan curve 40 percent"}); err != nil {
		t.Fatal(err)
	}
	if err := a.AttachMem(ctx, "src", "agent-b", "jwt"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := a.Recall(ctx, "agent-b", "jwt")
	if err != nil || !ok || got.Value != "use JWT on API" {
		t.Fatalf("attach recall: %+v ok=%v err=%v", got, ok, err)
	}
	if err := a.SliceMem(ctx, "src", "jwt-only", "jwt"); err != nil {
		t.Fatal(err)
	}
	hits, err := a.SearchMem(ctx, contract.MemoryQuery{NS: []string{"jwt-only"}, Text: "JWT"})
	if err != nil || len(hits) != 1 || hits[0].Key != "jwt" {
		t.Fatalf("slice hits=%+v err=%v", hits, err)
	}
	hits, err = a.SearchMem(ctx, contract.MemoryQuery{NS: []string{"jwt-only"}, Text: "fan"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("slice should hide fan, hits=%+v err=%v", hits, err)
	}
}

func TestE2EMemoryLinkAndWalk(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := linkMem(ctx, a, "lab", contract.MemoryEdge{From: "steve", Rel: "owns", To: "mi50"}); err != nil {
		t.Fatal(err)
	}
	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 1})
	if err != nil {
		t.Fatal(err)
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

func TestE2EMemoryLinkUnknownRelIgnored(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if err := linkMem(ctx, a, "lab", contract.MemoryEdge{From: "steve", Rel: "invented", To: "mi50"}); err != nil {
		t.Fatal(err)
	}
	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve"})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk) != 0 {
		t.Fatalf("unknown rel stored: %+v", walk)
	}
}

func TestE2EMemoryIngestCreatesEdge(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	edges, err := a.Ingest(ctx, "lab", "Steve owns the MI50. The MI50 is in the tower.")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges %+v", edges)
	}
	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 2})
	if err != nil {
		t.Fatal(err)
	}
	foundOwns := false
	for _, h := range walk {
		if h.Rel == "owns" && h.To == "mi50" {
			foundOwns = true
		}
	}
	if !foundOwns {
		t.Fatalf("walk %+v", walk)
	}
}

func TestE2EMemoryPolicyDenyRemember(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.conf")
	pol := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(model, []byte(`
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act, eft
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = (p.sub == "*" || p.sub == r.sub) && (p.obj == "*" || p.obj == r.obj || keyMatch(r.obj, p.obj)) && (p.act == "*" || p.act == r.act)
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pol, []byte("p, *, tracker, *, allow\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := e2eConfig(t)
	cfg.DataDir = filepath.Join(dir, "data")
	t.Setenv("IUGUM_DATA", cfg.DataDir)
	cfg.Policy.Model = model
	cfg.Policy.Policy = pol

	a := newE2EApp(t, cfg)
	err := a.Remember(context.Background(), contract.MemoryRec{Key: "k", Value: "v"})
	if err == nil {
		t.Fatal("want policy deny")
	}
	if _, ok := err.(contract.Denied); !ok {
		t.Fatalf("want Denied, got %T: %v", err, err)
	}
}
