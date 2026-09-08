package spaceassets

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if len(written) != 1 {
		t.Fatalf("Stage wrote %d files, want the one registered asset", len(written))
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

// The plug bytes are frozen at compile time, so Compare is the only way to see
// a forgotten rebuild. It has to report both numbers: the measured case served
// 101,481 bytes while the source held 132,357.
func TestCompareReportsBothByteCounts(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugs"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("plugs", "p.plug.js")
	if err := os.WriteFile(filepath.Join(root, src), bytes.Repeat([]byte("n"), 132357), 0o644); err != nil {
		t.Fatal(err)
	}
	Set([]Asset{{Rel: "Plugs/p.plug.js", Data: bytes.Repeat([]byte("o"), 101481), Src: filepath.ToSlash(src)}})

	sts := Compare(root)
	if len(sts) != 1 {
		t.Fatalf("Compare returned %d statuses, want 1", len(sts))
	}
	st := sts[0]
	if !st.Found {
		t.Fatal("Compare did not read a source file that is present")
	}
	if st.Current {
		t.Error("Compare called a 31KB gap current")
	}
	if st.Carried != 101481 || st.OnDisk != 132357 {
		t.Errorf("Compare measured carried=%d disk=%d, want 101481 and 132357", st.Carried, st.OnDisk)
	}
	if got := Stale(sts); len(got) != 1 {
		t.Errorf("Stale returned %d statuses, want the one behind asset", len(got))
	}
}

// A matching asset is the normal case. It must come back current, so nothing
// warns on a tree that was built a moment ago.
func TestCompareIsCurrentWhenTheBytesMatch(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("the same bytes")
	if err := os.WriteFile(filepath.Join(root, "plugs", "p.plug.js"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	Set([]Asset{{Rel: "Plugs/p.plug.js", Data: body, Src: "plugs/p.plug.js"}})

	sts := Compare(root)
	if len(sts) != 1 || !sts[0].Current {
		t.Fatalf("Compare called matching bytes stale: %+v", sts)
	}
	if got := Stale(sts); len(got) != 0 {
		t.Errorf("Stale returned %d statuses for a current tree", len(got))
	}
}

// Two files of one size can still differ. The gap is then invisible in the byte
// counts, so the status says so instead of printing two equal numbers alone.
func TestCompareFlagsEqualSizesWithDifferentContent(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugs", "p.plug.js"), []byte("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	Set([]Asset{{Rel: "Plugs/p.plug.js", Data: []byte("bbbb"), Src: "plugs/p.plug.js"}})

	sts := Compare(root)
	if len(sts) != 1 || sts[0].Current || !sts[0].SizeOnly {
		t.Fatalf("Compare missed a same-size difference: %+v", sts)
	}
}

// A source that is not there is not drift. The shipped binary runs on machines
// with no repository, and the check must never invent a fault there.
func TestCompareIsNotDriftWhenTheSourceIsAbsent(t *testing.T) {
	t.Cleanup(func() { Set(nil) })
	Set([]Asset{
		{Rel: "Plugs/p.plug.js", Data: []byte("x"), Src: "plugs/absent.plug.js"},
		{Rel: "Inline.md", Data: []byte("x")},
	})
	sts := Compare(t.TempDir())
	if len(sts) != 2 {
		t.Fatalf("Compare returned %d statuses, want 2", len(sts))
	}
	for _, st := range sts {
		if st.Found {
			t.Errorf("Compare claims to have read %q", st.Rel)
		}
	}
	if got := Stale(sts); len(got) != 0 {
		t.Errorf("Stale returned %d statuses with no sources present", len(got))
	}
}

// SourceRoot walks up from the working directory, so it works from any
// subdirectory of a checkout and finds nothing outside one.
func TestSourceRootWalksUpAndStopsOutsideACheckout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugs", "atomdown-inline"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(filepath.Join(root, "plugs", "atomdown-inline"))
	got, err := filepath.EvalSymlinks(SourceRoot())
	if err != nil {
		t.Fatalf("SourceRoot found no tree above a checkout subdirectory: %v", err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("SourceRoot found %q, want %q", got, want)
	}

	// A directory with no go.mod beside a plugs directory is not a checkout.
	bare := t.TempDir()
	t.Chdir(bare)
	if p := SourceRoot(); p != "" {
		t.Errorf("SourceRoot found %q outside a checkout", p)
	}
}

// The override exists for a checkout that is not above the working directory. A
// value that names no checkout is silence, not a guess.
func TestSourceRootHonoursTheOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plugs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSourceRoot, root)
	if got := SourceRoot(); got != root {
		t.Errorf("SourceRoot returned %q, want the override %q", got, root)
	}

	t.Setenv(EnvSourceRoot, filepath.Join(root, "absent"))
	if got := SourceRoot(); got != "" {
		t.Errorf("SourceRoot returned %q for an override that names no checkout", got)
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
