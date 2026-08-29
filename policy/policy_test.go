package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestDefaultAllowsScheduleAdd(t *testing.T) {
	g, err := New("", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Enforce(context.Background(), contract.Request{Sub: "scotty", Obj: "schedule", Act: "add"}); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyFileDenyScheduleAdd(t *testing.T) {
	dir := t.TempDir()
	pol := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(pol, []byte("p, *, *, *, allow\np, *, schedule, add, deny\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := New("", pol)
	if err != nil {
		t.Fatal(err)
	}
	err = g.Enforce(context.Background(), contract.Request{Sub: "scotty", Obj: "schedule", Act: "add"})
	if err == nil {
		t.Fatal("want deny")
	}
	if _, ok := err.(contract.Denied); !ok {
		t.Fatalf("want Denied, got %T %v", err, err)
	}
	if err := g.Enforce(context.Background(), contract.Request{Sub: "scotty", Obj: "schedule", Act: "list"}); err != nil {
		t.Fatalf("list should still allow: %v", err)
	}
}
