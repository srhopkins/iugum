package spaceassets

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllHoldsTheLibraryPages(t *testing.T) {
	Set(nil)
	assets, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(assets) == 0 {
		t.Fatal("All returned nothing; the library directory must hold at least one page")
	}
	var found bool
	for _, a := range assets {
		if a.Rel == "Editor Width.md" {
			found = true
			if !bytes.Contains(a.Data, []byte("space-lua")) {
				t.Error("the editor width page carries no space-lua block")
			}
		}
	}
	if !found {
		t.Error("All did not return the editor width page")
	}
}

func TestAllMergesSetAndSorts(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	Set([]Asset{
		{Rel: "Plugs/z.plug.js", Data: []byte("z")},
		{Rel: "Inline.md", Data: []byte("i")},
	})
	assets, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	for i := 1; i < len(assets); i++ {
		if assets[i-1].Rel > assets[i].Rel {
			t.Fatalf("All is not sorted: %q before %q", assets[i-1].Rel, assets[i].Rel)
		}
	}
	names := map[string]bool{}
	for _, a := range assets {
		names[a.Rel] = true
	}
	for _, want := range []string{"Plugs/z.plug.js", "Inline.md", "Editor Width.md"} {
		if !names[want] {
			t.Errorf("All dropped %q", want)
		}
	}
}

func TestStageWritesUnderTheNamespace(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	Set([]Asset{{Rel: "Plugs/atomdown-inline.plug.js", Data: []byte("plug")}})

	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "client_bundle", "base_fs"), 0o755); err != nil {
		t.Fatal(err)
	}
	written, err := Stage(src)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(written) < 2 {
		t.Fatalf("Stage wrote %d files, want the plug and the library pages", len(written))
	}
	for _, rel := range written {
		if !strings.HasPrefix(rel, Namespace+"/") {
			t.Errorf("Stage wrote %q outside %q", rel, Namespace)
		}
	}
	got, err := os.ReadFile(filepath.Join(src, "client_bundle", "base_fs", "Library", "Atomdown", "Plugs", "atomdown-inline.plug.js"))
	if err != nil {
		t.Fatalf("staged plug: %v", err)
	}
	if string(got) != "plug" {
		t.Errorf("staged plug holds %q, want %q", got, "plug")
	}
}

func TestStageRejectsATreeWithNoBaseFs(t *testing.T) {
	if _, err := Stage(t.TempDir()); err == nil {
		t.Fatal("Stage accepted a tree with no client_bundle/base_fs")
	}
}

func TestStageOverwritesAStaleAsset(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	src := t.TempDir()
	dir := filepath.Join(src, "client_bundle", "base_fs", "Library", "Atomdown", "Plugs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.plug.js"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	Set([]Asset{{Rel: "Plugs/p.plug.js", Data: []byte("new")}})
	if _, err := Stage(src); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "p.plug.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("Stage kept %q, want %q", got, "new")
	}
}

func TestBinaryHasAssetsFindsTheMarker(t *testing.T) {
	dir := t.TempDir()
	with := filepath.Join(dir, "with")
	body := append(bytes.Repeat([]byte("x"), 3<<20), []byte(Marker)...)
	if err := os.WriteFile(with, append(body, bytes.Repeat([]byte("y"), 1<<20)...), 0o755); err != nil {
		t.Fatal(err)
	}
	if !BinaryHasAssets(with) {
		t.Error("BinaryHasAssets missed a marker past the first chunk")
	}

	without := filepath.Join(dir, "without")
	if err := os.WriteFile(without, bytes.Repeat([]byte("x"), 2<<20), 0o755); err != nil {
		t.Fatal(err)
	}
	if BinaryHasAssets(without) {
		t.Error("BinaryHasAssets found a marker that is absent")
	}
}

func TestBinaryHasAssetsReportsTrueWhenItCannotRead(t *testing.T) {
	if !BinaryHasAssets(filepath.Join(t.TempDir(), "absent")) {
		t.Error("a check that cannot run must not report a missing asset set")
	}
}

// chokeReader hands out at most n bytes per Read, so a test can force the
// needle to straddle two reads.
type chokeReader struct {
	r io.Reader
	n int
}

func (c chokeReader) Read(p []byte) (int, error) {
	if len(p) > c.n {
		p = p[:c.n]
	}
	return c.r.Read(p)
}

func TestContainsMarkerAcrossAReadBoundary(t *testing.T) {
	needle := []byte("ABCDEFGH")
	body := append(bytes.Repeat([]byte("."), 10), needle...)
	body = append(body, bytes.Repeat([]byte("."), 10)...)
	// Reads of 12 bytes split the needle: it starts at offset 10.
	found, err := containsMarker(chokeReader{r: bytes.NewReader(body), n: 12}, needle)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("containsMarker missed a needle across a read boundary")
	}
}
