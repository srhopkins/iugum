package spaceseed

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAssets() []Asset {
	return []Asset{
		{Rel: "_plug/one.plug.js", Data: []byte("plug one\n")},
		{Rel: "Library/Test Page.md", Data: []byte("# page\n")},
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestSeedEmptySpaceWritesEverything(t *testing.T) {
	dir := t.TempDir()
	var log bytes.Buffer
	res, err := SeedAssets(dir, testAssets(), Options{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wrote) != 2 {
		t.Fatalf("wrote %v, want two files", res.Wrote)
	}
	if got := read(t, filepath.Join(dir, "_plug", "one.plug.js")); got != "plug one\n" {
		t.Fatalf("plug content %q", got)
	}
	if got := read(t, filepath.Join(dir, "Library", "Test Page.md")); got != "# page\n" {
		t.Fatalf("page content %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	if !res.Changed() {
		t.Fatal("Changed is false after a write")
	}
	if !strings.Contains(log.String(), "Space: Reindex") {
		t.Fatalf("log has no reindex instruction: %q", log.String())
	}
}

func TestSecondRunIsSilent(t *testing.T) {
	dir := t.TempDir()
	if _, err := SeedAssets(dir, testAssets(), Options{}); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	res, err := SeedAssets(dir, testAssets(), Options{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed() || len(res.Skipped) != 0 {
		t.Fatalf("second run acted: %+v", res)
	}
	if len(res.Current) != 2 {
		t.Fatalf("current %v, want two files", res.Current)
	}
	if log.Len() != 0 {
		t.Fatalf("second run said %q, want silence", log.String())
	}
}

func TestChangedFileIsKeptAndReported(t *testing.T) {
	dir := t.TempDir()
	if _, err := SeedAssets(dir, testAssets(), Options{}); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "Library", "Test Page.md")
	if err := os.WriteFile(page, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	res, err := SeedAssets(dir, testAssets(), Options{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, page); got != "# mine\n" {
		t.Fatalf("the seeder overwrote a changed file: %q", got)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "Library/Test Page.md" {
		t.Fatalf("skipped %v", res.Skipped)
	}
	if res.Changed() {
		t.Fatal("Changed is true with nothing written")
	}
	if !strings.Contains(log.String(), "kept Library/Test Page.md") {
		t.Fatalf("log has no warning: %q", log.String())
	}
	if strings.Contains(log.String(), "Space: Reindex") {
		t.Fatalf("log asks for a reindex with nothing written: %q", log.String())
	}
}

func TestForceTakesTheBuiltInCopy(t *testing.T) {
	dir := t.TempDir()
	if _, err := SeedAssets(dir, testAssets(), Options{}); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(dir, "Library", "Test Page.md")
	if err := os.WriteFile(page, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := SeedAssets(dir, testAssets(), Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, page); got != "# page\n" {
		t.Fatalf("force left %q", got)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("updated %v", res.Updated)
	}
}

func TestAnOlderSeedIsUpdatedAndNamedStale(t *testing.T) {
	dir := t.TempDir()
	old := []Asset{
		{Rel: "_plug/one.plug.js", Data: []byte("plug one\n")},
		{Rel: "Library/Test Page.md", Data: []byte("# old page\n")},
	}
	if _, err := SeedAssets(dir, old, Options{}); err != nil {
		t.Fatal(err)
	}
	// Pretend the record came from an older program.
	man := read(t, filepath.Join(dir, ManifestName))
	man = strings.Replace(man, `"iugum_seed_version": 1`, `"iugum_seed_version": 0`, 1)
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(man), 0o644); err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	res, err := SeedAssets(dir, testAssets(), Options{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "Library", "Test Page.md")); got != "# page\n" {
		t.Fatalf("stale page not updated: %q", got)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("updated %v", res.Updated)
	}
	if !strings.Contains(log.String(), "older iugum") {
		t.Fatalf("log does not name the older seed: %q", log.String())
	}
}

func TestPresentButUnrecordedFileIsNeverTouched(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "Library", "Test Page.md")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("# hand written\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := SeedAssets(dir, testAssets(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, page); got != "# hand written\n" {
		t.Fatalf("a hand-written page was overwritten: %q", got)
	}
	if len(res.Skipped) != 1 || len(res.Wrote) != 1 {
		t.Fatalf("res %+v", res)
	}
}

func TestOnceRunsOnePerSpace(t *testing.T) {
	dir := t.TempDir()
	Set(testAssets())
	t.Cleanup(func() { Set(nil) })
	res, err := Seed(dir, Options{Once: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wrote) != 2 {
		t.Fatalf("first run wrote %v", res.Wrote)
	}
	if err := os.Remove(filepath.Join(dir, "_plug", "one.plug.js")); err != nil {
		t.Fatal(err)
	}
	res, err = Seed(dir, Options{Once: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wrote) != 0 {
		t.Fatalf("the second Once run acted: %v", res.Wrote)
	}
}

func TestNoAssetsIsANoOp(t *testing.T) {
	dir := t.TempDir()
	res, err := SeedAssets(dir, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed() {
		t.Fatal("an empty asset set changed the space")
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); !os.IsNotExist(err) {
		t.Fatalf("an empty asset set wrote a manifest: %v", err)
	}
}
