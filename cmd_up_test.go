package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

func TestContainerRunArgvDocker(t *testing.T) {
	o := upOpts{Container: true, Detach: true, Name: "demo"}
	got := strings.Join(containerRunArgv("docker", config.Container{}, o, "/w", "/d"), " ")
	for _, want := range []string{
		"docker run --rm -d --name demo",
		"-v /w:/workspace", "-v /d:/data",
		"-p 3000:3000", "-p 3848:3848", "-p 8080:8080",
		"-e IUGUM_DATA=/data",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv %q lacks %q", got, want)
		}
	}
	if !strings.HasSuffix(got, " iugum:latest up") {
		t.Fatalf("argv %q must end with image then up", got)
	}
}

func TestContainerRunArgvPodmanConfig(t *testing.T) {
	c := config.Container{
		Image: "ghcr.io/x/y:1", Name: "cfgname",
		Mounts: []string{"/m:/m"}, Ports: []string{"9000:3000"}, Env: []string{"A=b"},
	}
	got := strings.Join(containerRunArgv("podman", c, upOpts{}, "/w", "/d"), " ")
	if !strings.HasPrefix(got, "podman run --rm --name cfgname ") {
		t.Fatalf("prefix wrong: %q", got)
	}
	for _, want := range []string{"-v /m:/m", "-p 9000:3000", "-e A=b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "3848") {
		t.Fatalf("config ports must replace defaults: %q", got)
	}
	if !strings.HasSuffix(got, " ghcr.io/x/y:1 up") {
		t.Fatalf("argv %q must end with config image then up", got)
	}
}

func TestContainerBuildStopArgv(t *testing.T) {
	b := strings.Join(containerBuildArgv("docker", config.Container{}, containerOpts{With: "claude,codex"}), " ")
	if b != "docker build --build-arg WITH=claude,codex -t iugum:latest ." {
		t.Fatalf("build argv: %q", b)
	}
	b2 := strings.Join(containerBuildArgv("docker", config.Container{}, containerOpts{With: "opencode", CodeServer: "1", Browser: "1"}), " ")
	if b2 != "docker build --build-arg WITH=opencode --build-arg CODE_SERVER=1 --build-arg BROWSER=1 -t iugum:latest ." {
		t.Fatalf("build argv with overlays: %q", b2)
	}
	s := strings.Join(containerStopArgv("podman", config.Container{}, containerOpts{}), " ")
	if s != "podman stop iugum" {
		t.Fatalf("stop argv: %q", s)
	}
}

func TestResolveEngineAuto(t *testing.T) {
	dir := t.TempDir()
	fake := func(name string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	if _, err := resolveEngine("auto", "", false); err == nil {
		t.Fatal("empty PATH must fail auto")
	}
	if e, err := resolveEngine("auto", "", true); err != nil || e != "docker" {
		t.Fatalf("dry-run auto on empty PATH = %q, %v", e, err)
	}
	fake("podman")
	if e, err := resolveEngine("auto", "", false); err != nil || e != "podman" {
		t.Fatalf("auto with podman only = %q, %v", e, err)
	}
	fake("docker")
	if e, err := resolveEngine("", "auto", false); err != nil || e != "docker" {
		t.Fatalf("auto prefers docker = %q, %v", e, err)
	}
	if e, err := resolveEngine("", "podman", false); err != nil || e != "podman" {
		t.Fatalf("config engine wins = %q, %v", e, err)
	}
	if _, err := resolveEngine("rkt", "", true); err == nil || !strings.Contains(err.Error(), "engine") {
		t.Fatalf("unknown engine must fail with 'engine': %v", err)
	}
}

func TestParseUpArgs(t *testing.T) {
	o, code, ok := parseUpArgs([]string{"--container", "--engine", "podman", "--image=x:1", "--name", "n", "--detach", "--dry-run"})
	if !ok || code != 0 {
		t.Fatalf("parse failed: code=%d", code)
	}
	if !o.Container || o.Engine != "podman" || o.Image != "x:1" || o.Name != "n" || !o.Detach || !o.DryRun {
		t.Fatalf("opts wrong: %+v", o)
	}
	o, _, ok = parseUpArgs([]string{"--wiki-port", "0", "--observe-port=0", "--no-code-server"})
	if !ok || o.WikiPort != 0 || o.ObservePort != 0 || !o.NoCodeServer || o.CodeServerPort != 8080 {
		t.Fatalf("host opts wrong: %+v", o)
	}
	if _, code, ok := parseUpArgs([]string{"--bogus"}); ok || code != 2 {
		t.Fatal("unknown flag must fail with 2")
	}
}

func TestDenyPath(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.conf")
	pol := filepath.Join(dir, "policy.csv")
	os.WriteFile(model, []byte(`[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.sub == "*" || p.sub == r.sub) && (p.obj == "*" || p.obj == r.obj) && (p.act == "*" || p.act == r.act)
`), 0o644)
	os.WriteFile(pol, []byte("p, *, *, *, allow\np, *, container, run, deny\np, *, service, serve, deny\n"), 0o644)
	t.Setenv("IUGUM_DATA", dir)
	cfg := config.Defaults()
	cfg.Policy = config.Policy{Model: model, Policy: pol}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var d contract.Denied
	if err := a.Check(ctx, "container", "run"); !errors.As(err, &d) {
		t.Fatalf("want Denied, got %v", err)
	}
	if code := runUpContainer(ctx, a, cfg, upOpts{Container: true, Engine: "docker", DryRun: true}); code != 1 {
		t.Fatalf("deny container run must exit 1, got %d", code)
	}
	if code := runUpHost(ctx, a, cfg, upOpts{WikiPort: 0, ObservePort: 0, NoCodeServer: true, Host: "127.0.0.1"}); code != 1 {
		t.Fatalf("deny service serve must exit 1, got %d", code)
	}
	if err := a.Check(ctx, "container", "build"); err != nil {
		t.Fatalf("build must stay allowed: %v", err)
	}
}
