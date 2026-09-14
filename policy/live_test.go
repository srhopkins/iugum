package policy

import (
	"context"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveRevocation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.csv")
	write := func(s string) {
		if e := os.WriteFile(p, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("p, agent:a, search, read, allow\n")
	g := Live{Path: p}
	r := contract.Request{Sub: "agent:a", Obj: "search", Act: "read"}
	if e := g.Enforce(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	write("p, agent:a, search, read, deny\n")
	if e := g.Enforce(context.Background(), r); e == nil {
		t.Fatal("revocation ignored")
	}
	os.Remove(p)
	if e := g.Enforce(context.Background(), r); e == nil {
		t.Fatal("missing policy allowed access")
	}
}
func TestNamedRoleGrantAndRevoke(t *testing.T) {
	p := filepath.Join(t.TempDir(), "policy.csv")
	os.WriteFile(p, []byte("p, reader, wiki*, read, allow\ng, agent:a, reader\n"), 0600)
	g := Live{Path: p}
	r := contract.Request{Sub: "agent:a", Obj: "wiki:notes.md", Act: "read"}
	if e := g.Enforce(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	r.Act = "write"
	if e := g.Enforce(context.Background(), r); e == nil {
		t.Fatal("role allowed write")
	}
	os.WriteFile(p, []byte("p, reader, wiki*, read, allow\n"), 0600)
	r.Act = "read"
	if e := g.Enforce(context.Background(), r); e == nil {
		t.Fatal("revoked role still allowed")
	}
}
