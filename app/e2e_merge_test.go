package app

import (
	"context"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestE2EIngestMergeOnePersonRoot(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	ctx := context.Background()

	if _, err := a.Ingest(ctx, "lab", "Steve owns the MI50."); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Ingest(ctx, "lab", "Steve Hopkins owns the MI50."); err != nil {
		t.Fatal(err)
	}

	walk, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve", Hops: 1, Rel: "owns"})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk) != 1 || walk[0].To != "mi50" {
		t.Fatalf("want one owns edge from steve to mi50, got %+v", walk)
	}

	alt, err := a.Walk(ctx, contract.WalkQuery{NS: "lab", From: "steve-hopkins", Hops: 1, Rel: "owns"})
	if err != nil {
		t.Fatal(err)
	}
	if len(alt) != 0 {
		t.Fatalf("steve-hopkins should not be a separate walk root, got %+v", alt)
	}
}
