package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/adapter/net/iptables"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/policy"
)

const denyModel = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.sub == "*" || p.sub == r.sub) && (p.obj == "*" || p.obj == r.obj || keyMatch(r.obj, p.obj)) && (p.act == "*" || p.act == r.act)
`

// netApp builds a minimal App with the iptables backend and an optional deny row.
func netApp(t *testing.T, denyAct string) *app.App {
	t.Helper()
	model, pol := "", ""
	if denyAct != "" {
		dir := t.TempDir()
		model = filepath.Join(dir, "model.conf")
		pol = filepath.Join(dir, "policy.csv")
		if err := os.WriteFile(model, []byte(denyModel), 0o644); err != nil {
			t.Fatal(err)
		}
		rows := "p, *, *, *, allow\np, *, net, " + denyAct + ", deny\n"
		if err := os.WriteFile(pol, []byte(rows), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gate, err := policy.New(model, pol)
	if err != nil {
		t.Fatal(err)
	}
	return &app.App{Actor: "test", Gate: gate, Net: &iptables.Net{}, NetRules: iptables.Fixture()}
}

func TestNetPlanPrintsRules(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := runNetIO(context.Background(), netApp(t, ""), []string{"plan"}, &out, &errb); rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "iptables -A INPUT -p tcp --dport 3000 -s 203.0.113.0/24 -j ACCEPT") {
		t.Fatalf("plan output:\n%s", out.String())
	}
}

func TestNetApplyDenied(t *testing.T) {
	var out, errb bytes.Buffer
	a := netApp(t, "apply")
	if rc := runNetIO(context.Background(), a, []string{"apply", "--dry-run"}, &out, &errb); rc != 1 {
		t.Fatalf("rc=%d want 1", rc)
	}
	if !strings.Contains(errb.String(), "denied") {
		t.Fatalf("stderr=%s", errb.String())
	}
	out.Reset()
	if rc := runNetIO(context.Background(), a, []string{"plan"}, &out, &errb); rc != 0 {
		t.Fatalf("plan should still pass, rc=%d", rc)
	}
}

func TestNetApplyLinuxOnly(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("darwin/other OS only")
	}
	var out, errb bytes.Buffer
	if rc := runNetIO(context.Background(), netApp(t, ""), []string{"apply"}, &out, &errb); rc != 1 {
		t.Fatalf("rc=%d want 1", rc)
	}
	if !strings.Contains(strings.ToLower(errb.String()), "linux") {
		t.Fatalf("stderr=%s", errb.String())
	}
}

func TestNetOffIsNoop(t *testing.T) {
	a := netApp(t, "")
	a.Net = nil
	var out, errb bytes.Buffer
	if rc := runNetIO(context.Background(), a, []string{"plan"}, &out, &errb); rc != 0 || out.Len() != 0 {
		t.Fatalf("rc=%d out=%q", rc, out.String())
	}
	if err := a.ApplyNet(context.Background()); err != nil {
		t.Fatal(err)
	}
}
