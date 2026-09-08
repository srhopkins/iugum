package sbadapt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/spaceassets"
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

// writePlugSource writes one plug source of n bytes under root and registers a
// carried asset of carried bytes for it, so a test can set the size of the gap.
func writePlugSource(t *testing.T, root, src string, carried, onDisk int) spaceassets.Asset {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(src))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, bytes.Repeat([]byte("n"), onDisk), 0o644); err != nil {
		t.Fatal(err)
	}
	return spaceassets.Asset{
		Rel:  "Plugs/atomdown-inline.plug.js",
		Data: bytes.Repeat([]byte("o"), carried),
		Src:  src,
	}
}

// The plugs are compiled in, so a forgotten rebuild is invisible at run time.
// The warning has to name both byte counts and the one command that closes the
// gap: a wiki served 101,481 bytes while the source held 132,357, and that read
// as a rendering bug for an evening.
func TestWarnIfStalePlugsNamesBothByteCountsAndTheFix(t *testing.T) {
	t.Cleanup(func() { spaceassets.Set(nil) })
	root := t.TempDir()
	asset := writePlugSource(t, root, "plugs/atomdown-inline/atomdown-inline.plug.js", 101481, 132357)
	spaceassets.Set([]spaceassets.Asset{asset})

	var out bytes.Buffer
	warnIfStalePlugs(&out, root)
	got := out.String()

	for _, want := range []string{
		"101481",
		"132357",
		"plugs/atomdown-inline/atomdown-inline.plug.js",
		"scripts/build-wiki-blob.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warning does not mention %q:\n%s", want, got)
		}
	}
}

// A tree that matches is the normal case and stays silent, and so does a binary
// with no tree beside it. The shipped binary runs on machines with no sources,
// where a warning would be wrong, and a warning on a fresh build would train the
// reader to ignore this one.
func TestWarnIfStalePlugsStaysSilent(t *testing.T) {
	t.Cleanup(func() { spaceassets.Set(nil) })

	var noTree bytes.Buffer
	spaceassets.Set([]spaceassets.Asset{{Rel: "Plugs/p.plug.js", Data: []byte("x"), Src: "plugs/p.plug.js"}})
	warnIfStalePlugs(&noTree, "")
	if noTree.Len() != 0 {
		t.Errorf("warned with no source tree to compare against:\n%s", noTree.String())
	}

	// A tree that is present but holds no such file is also silence.
	var noSource bytes.Buffer
	warnIfStalePlugs(&noSource, t.TempDir())
	if noSource.Len() != 0 {
		t.Errorf("warned about a source file that is absent:\n%s", noSource.String())
	}

	root := t.TempDir()
	same := writePlugSource(t, root, "plugs/atomdown-inline/atomdown-inline.plug.js", 0, 4096)
	same.Data = bytes.Repeat([]byte("n"), 4096)
	spaceassets.Set([]spaceassets.Asset{same})

	var current bytes.Buffer
	warnIfStalePlugs(&current, root)
	if current.Len() != 0 {
		t.Errorf("warned about a plug that matches its source:\n%s", current.String())
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
