package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/srhopkins/iugum/spaceseed"
)

// The program must carry every file a space needs. A space with the plugs and
// no library page draws the decorations with no stylesheet.
func TestEmbeddedSpaceAssets(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"board plug", atomdownBoardPlug, "Atomdown: Toggle Board"},
		{"inline plug", atomdownInlinePlug, "Atomdown: Toggle Inline View"},
		{"library page", atomdownInlineLibrary, "space-style"},
	} {
		if len(tc.data) == 0 {
			t.Fatalf("%s is empty", tc.name)
		}
		if !bytes.Contains(tc.data, []byte(tc.want)) {
			t.Fatalf("%s does not carry %q", tc.name, tc.want)
		}
	}
	if !bytes.Contains(atomdownInlineLibrary, []byte("space-lua")) {
		t.Fatal("the library page carries no space-lua, so it defines no header button")
	}
}

// The registered asset set seeds an empty space with all three files.
func TestSeedFromEmbeddedAssets(t *testing.T) {
	dir := t.TempDir()
	res, err := spaceseed.Seed(dir, spaceseed.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"_plug/atomdown-board.plug.js",
		"_plug/atomdown-inline.plug.js",
		"Library/Atomdown Inline.md",
	}
	if len(res.Wrote) != len(want) {
		t.Fatalf("wrote %v, want %v", res.Wrote, want)
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
	}
}
