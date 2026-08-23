package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	_ "github.com/srhopkins/iugum/defaults"
	"github.com/srhopkins/iugum/policy"
)

func TestRememberThroughAllowAll(t *testing.T) {
	t.Setenv("IUGUM_DATA", t.TempDir())
	a, err := New(config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.Remember(ctx, contract.MemoryRec{Key: "k", Value: "v"}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := a.Recall(ctx, "", "k")
	if err != nil || !ok || got.Value != "v" {
		t.Fatalf("%+v ok=%v err=%v", got, ok, err)
	}
}

func TestMemoryPolicyDeny(t *testing.T) {
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
	g, err := policy.New(model, pol)
	if err != nil {
		t.Fatal(err)
	}
	err = g.Enforce(context.Background(), contract.Request{Sub: "steve", Obj: contract.MemoryObj("fact", "default"), Act: "write"})
	if err == nil {
		t.Fatal("want deny")
	}
}

func TestMemoryObjShape(t *testing.T) {
	if got := contract.MemoryObj("fact", "steve/agent"); got != "mem/fact/ns/steve/agent" {
		t.Fatal(got)
	}
}
