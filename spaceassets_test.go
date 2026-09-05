package main

import (
	"bytes"
	"testing"

	"github.com/srhopkins/iugum/spaceassets"
)

// The program must carry every file a space needs. A space with the plugs and
// no library page draws the decorations with no stylesheet, which reads as a
// broken feature rather than an absent one.
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

// init registers the embedded assets, so the staging step and the marker check
// both see the full set.
func TestRegisteredAssetSet(t *testing.T) {
	assets, err := spaceassets.All()
	if err != nil {
		t.Fatal(err)
	}
	byRel := map[string]int{}
	for _, a := range assets {
		byRel[a.Rel] = len(a.Data)
	}
	want := []string{
		"Plugs/atomdown-board.plug.js",
		"Plugs/atomdown-inline.plug.js",
		"Inline.md",
	}
	for _, rel := range want {
		if byRel[rel] == 0 {
			t.Errorf("the asset set has no %q", rel)
		}
	}
	// The bundle stays limited to what the atomdown views need. Anything else
	// reaches every space with no way to switch it off, so a new asset is a
	// deliberate decision, not a drive-by addition.
	if len(assets) != len(want) {
		t.Errorf("the asset set holds %d files, want %d: %v", len(assets), len(want), assets)
	}
}

// The marker names a path that the set really stages. A marker that no asset
// produces would report every SilverBullet binary as stale.
func TestMarkerNamesAStagedAsset(t *testing.T) {
	assets, err := spaceassets.All()
	if err != nil {
		t.Fatal(err)
	}
	want := spaceassets.Marker
	for _, a := range assets {
		if spaceassets.Namespace+"/"+a.Rel == want {
			return
		}
	}
	t.Fatalf("no asset stages the marker path %q", want)
}
