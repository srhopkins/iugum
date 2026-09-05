package sbadapt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A hand-copied plug bundle in _plug is not an override. SilverBullet's own
// Space.listPlugs loads every *.plug.js the space can see, and the same plugs
// are compiled into the binary, so the copy makes the plug run TWICE. Two
// instances with two memories, both writing one config key: collapsing eleven
// groups and expanding them again left nine shut, and which nine moved between
// runs.
func TestWarnIfDuplicatePlugsNamesEveryCopy(t *testing.T) {
	space := t.TempDir()
	dir := filepath.Join(space, "_plug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"atomdown-inline.plug.js",
		"atomdown-board.plug.js",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	warnIfDuplicatePlugs(&out, space)
	got := out.String()

	for _, want := range []string{
		"atomdown-inline.plug.js",
		"atomdown-board.plug.js",
		"runs twice",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning does not mention %q:\n%s", want, got)
		}
	}
}

// Silence is the normal case, and it has to be silent: a space that never had a
// copy, and a space that overrides the compiled plug at its own path, are both
// correct. A warning on either would train the reader to ignore this one.
func TestWarnIfDuplicatePlugsStaysSilent(t *testing.T) {
	space := t.TempDir()

	var empty bytes.Buffer
	warnIfDuplicatePlugs(&empty, space)
	if empty.Len() != 0 {
		t.Errorf("warned about a space with no _plug directory:\n%s", empty.String())
	}

	// An override at the compiled path is deliberate: a space file of the same
	// name wins over the binary's underlay, so there is still one copy.
	override := filepath.Join(space, "Library", "Atomdown", "Plugs")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(override, "atomdown-inline.plug.js")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// And an unrelated plug in _plug is somebody else's, not a duplicate.
	other := filepath.Join(space, "_plug")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "index.plug.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var quiet bytes.Buffer
	warnIfDuplicatePlugs(&quiet, space)
	if quiet.Len() != 0 {
		t.Errorf("warned about an override and an unrelated plug:\n%s", quiet.String())
	}
}
