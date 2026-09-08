package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/policy"
	"github.com/srhopkins/iugum/spaceassets"
)

// restoreAssets puts the program's own assets back after a test replaces them.
// The registry is process-wide, and spaceassets_test.go reads it.
func restoreAssets(t *testing.T) {
	t.Helper()
	before, err := spaceassets.All()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { spaceassets.Set(before) })
}

func checkTestApp(t *testing.T) *app.App {
	t.Helper()
	gate, err := policy.New("", "")
	if err != nil {
		t.Fatal(err)
	}
	return &app.App{Actor: "test", Gate: gate}
}

// writeAssetTree registers one carried asset and writes its source file, so a
// test can set the size of the gap between the two.
func writeAssetTree(t *testing.T, carried, onDisk int) string {
	t.Helper()
	root := t.TempDir()
	src := "plugs/atomdown-inline/atomdown-inline.plug.js"
	full := filepath.Join(root, filepath.FromSlash(src))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("n"), onDisk)
	if err := os.WriteFile(full, body, 0o644); err != nil {
		t.Fatal(err)
	}
	data := body
	if carried != onDisk {
		data = bytes.Repeat([]byte("o"), carried)
	}
	spaceassets.Set([]spaceassets.Asset{{
		Rel:  "Plugs/atomdown-inline.plug.js",
		Data: data,
		Src:  src,
	}})
	return root
}

// The command exists so nobody compares two byte counts by eye. It has to print
// both numbers, mark the asset as behind, name the fix, and exit non-zero.
func TestCheckWikiAssetsReportsAStalePlug(t *testing.T) {
	restoreAssets(t)
	root := writeAssetTree(t, 101481, 132357)

	var out, errb bytes.Buffer
	code := runCheckWikiAssetsIO(context.Background(), checkTestApp(t), []string{root}, &out, &errb)
	if code != 1 {
		t.Fatalf("exit %d for a stale plug, want 1. stderr: %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{
		"Plugs/atomdown-inline.plug.js",
		"101481",
		"132357",
		"BEHIND",
		"scripts/build-wiki-blob.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not mention %q:\n%s", want, got)
		}
	}
}

// A current tree exits 0 and says so in one line, so the answer needs no
// reading of the table above it.
func TestCheckWikiAssetsReportsACurrentTree(t *testing.T) {
	restoreAssets(t)
	root := writeAssetTree(t, 4096, 4096)

	var out, errb bytes.Buffer
	code := runCheckWikiAssetsIO(context.Background(), checkTestApp(t), []string{root}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d for a current tree, want 0. stderr: %s", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "current") {
		t.Errorf("output does not say the assets are current:\n%s", got)
	}
	if strings.Contains(got, "BEHIND") {
		t.Errorf("output calls a current asset behind:\n%s", got)
	}
}

// With no tree to compare against the command is not a fault: it says there is
// nothing to compare and exits 0, the same way the startup warning is silent.
func TestCheckWikiAssetsWithNoTreeIsNotAFault(t *testing.T) {
	restoreAssets(t)
	spaceassets.Set([]spaceassets.Asset{{
		Rel:  "Plugs/atomdown-inline.plug.js",
		Data: []byte("x"),
		Src:  "plugs/atomdown-inline/atomdown-inline.plug.js",
	}})
	bare := t.TempDir()
	t.Chdir(bare)
	t.Setenv(spaceassets.EnvSourceRoot, "")

	var out, errb bytes.Buffer
	code := runCheckWikiAssetsIO(context.Background(), checkTestApp(t), nil, &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d with no source tree, want 0. stderr: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "nothing to compare") {
		t.Errorf("output does not say there is nothing to compare:\n%s", out.String())
	}
}

func TestCheckWikiAssetsRejectsExtraArguments(t *testing.T) {
	var out, errb bytes.Buffer
	code := runCheckWikiAssetsIO(context.Background(), checkTestApp(t), []string{"a", "b"}, &out, &errb)
	if code != 2 {
		t.Fatalf("exit %d for two directories, want 2", code)
	}
	if !strings.Contains(errb.String(), "Usage:") {
		t.Errorf("stderr does not carry usage:\n%s", errb.String())
	}
}
