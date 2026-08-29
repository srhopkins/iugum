package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/policy"
)

func TestParseJobAddSession(t *testing.T) {
	j, err := parseJobAdd([]string{"hourly-checks", "--every", "1h", "--prompt", "Run hourly-checks"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Name != "hourly-checks" || j.Kind != "session" || j.Spec != "@every 1h" || j.Prompt != "Run hourly-checks" {
		t.Fatalf("got %+v", j)
	}
}

func TestParseJobAddExec(t *testing.T) {
	j, err := parseJobAdd([]string{"ping", "--every", "5m", "--", "echo", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != "exec" || j.Spec != "@every 5m" || len(j.Command) != 2 || j.Command[0] != "echo" {
		t.Fatalf("got %+v", j)
	}
}

func TestJobAddListRemove(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("IUGUM_JOBS", filepath.Join(dir, "jobs.yaml"))
	t.Setenv("IUGUM_DATA", dir)

	a := jobTestApp(t, "")
	ctx := context.Background()
	var out, errb bytes.Buffer
	if code := runJobIO(ctx, a, []string{"add", "hourly-checks", "--every", "1h", "--prompt", "Run hourly-checks"}, &out, &errb); code != 0 {
		t.Fatalf("add: %d %s", code, errb.String())
	}
	out.Reset()
	if code := runJobIO(ctx, a, []string{"ls"}, &out, &errb); code != 0 {
		t.Fatalf("ls: %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "hourly-checks") || !strings.Contains(out.String(), "@every 1h") {
		t.Fatalf("ls = %q", out.String())
	}
	out.Reset()
	if code := runJobIO(ctx, a, []string{"rm", "hourly-checks"}, &out, &errb); code != 0 {
		t.Fatalf("rm: %d %s", code, errb.String())
	}
	jobs, err := config.LoadJobsFile(filepath.Join(dir, "jobs.yaml"))
	if err != nil || len(jobs) != 0 {
		t.Fatalf("after rm: %+v %v", jobs, err)
	}
}

func TestJobAddDeniedByPolicy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("IUGUM_JOBS", filepath.Join(dir, "jobs.yaml"))
	t.Setenv("IUGUM_DATA", dir)

	a := jobTestApp(t, "add")
	var out, errb bytes.Buffer
	if code := runJobIO(context.Background(), a, []string{"add", "nope", "--every", "1h", "--prompt", "x"}, &out, &errb); code != 1 {
		t.Fatalf("want deny exit 1, got %d %s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "denied") {
		t.Fatalf("want denied, got %q", errb.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "jobs.yaml")); !os.IsNotExist(err) {
		t.Fatal("denied add must not write jobs.yaml")
	}
}

func jobTestApp(t *testing.T, denyAct string) *app.App {
	t.Helper()
	model, pol := "", ""
	if denyAct != "" {
		dir := t.TempDir()
		pol = filepath.Join(dir, "policy.csv")
		if err := os.WriteFile(pol, []byte("p, *, *, *, allow\np, *, schedule, "+denyAct+", deny\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gate, err := policy.New(model, pol)
	if err != nil {
		t.Fatal(err)
	}
	return &app.App{Actor: "test", Gate: gate}
}
