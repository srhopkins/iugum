// Package spaceseed writes the atomdown runtime assets into a SilverBullet
// space. The assets are compiled into the program; root package main embeds
// them and calls Set, the same way embedbin holds the SilverBullet binary.
//
// The seeder is additive. It writes a file that is absent, it leaves a file
// that is byte-identical, and it never overwrites a file that somebody
// changed. A manifest in the space records the digest of each file this
// program wrote, so a space seeded by an older iugum is detectable.
package spaceseed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Version counts the asset set. Raise it when an asset changes, so an older
// seed reads as stale in the log.
const Version = 1

// ManifestName is the record of what this program wrote, in the space root.
// The name starts with a dot, so SilverBullet does not index it as a page.
const ManifestName = ".iugum-seed.json"

// Asset is one file to seed. Rel is the path in the space, with forward
// slashes.
type Asset struct {
	Rel  string
	Data []byte
}

var (
	mu     sync.Mutex
	assets []Asset
	// seeded holds the space paths this process already handled, so the
	// supervised restart loop in `iugum up` does not repeat the log.
	seeded = map[string]bool{}
)

// Set records the compiled-in asset set. Root package main calls it.
func Set(a []Asset) {
	mu.Lock()
	defer mu.Unlock()
	assets = a
}

// Options control one seed run.
type Options struct {
	// Force rewrites a changed file instead of warning about it.
	Force bool
	// Log takes one line per action. A nil Log discards the lines.
	Log io.Writer
	// Once skips a space this process already seeded.
	Once bool
}

// Result says what a run did.
type Result struct {
	Wrote    []string
	Updated  []string
	Skipped  []string
	Current  []string
	WasStale bool
}

// Changed reports whether the run touched the space. A caller prints the
// reindex instruction only then.
func (r Result) Changed() bool { return len(r.Wrote) > 0 || len(r.Updated) > 0 }

// Seed writes the compiled-in assets into the space at dir.
func Seed(dir string, opts Options) (Result, error) {
	mu.Lock()
	set := assets
	mu.Unlock()
	if opts.Once {
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		mu.Lock()
		done := seeded[abs]
		seeded[abs] = true
		mu.Unlock()
		if done {
			return Result{}, nil
		}
	}
	return SeedAssets(dir, set, opts)
}

// SeedAssets is Seed with an explicit asset set. Tests use it.
func SeedAssets(dir string, set []Asset, opts Options) (Result, error) {
	var res Result
	if len(set) == 0 {
		return res, nil
	}
	logf := func(format string, a ...any) {
		if opts.Log == nil {
			return
		}
		fmt.Fprintf(opts.Log, format+"\n", a...)
	}
	man, err := readManifest(dir)
	if err != nil {
		return res, err
	}
	if man.Version != 0 && man.Version < Version {
		res.WasStale = true
	}
	next := manifest{Version: Version, Assets: map[string]string{}}
	for _, a := range set {
		want := digest(a.Data)
		path := filepath.Join(dir, filepath.FromSlash(a.Rel))
		have, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := write(path, a.Data); err != nil {
				return res, err
			}
			next.Assets[a.Rel] = want
			res.Wrote = append(res.Wrote, a.Rel)
			logf("wiki: seeded %s", a.Rel)
			continue
		case err != nil:
			return res, err
		}
		switch {
		case digest(have) == want:
			next.Assets[a.Rel] = want
			res.Current = append(res.Current, a.Rel)
		case opts.Force:
			if err := write(path, a.Data); err != nil {
				return res, err
			}
			next.Assets[a.Rel] = want
			res.Updated = append(res.Updated, a.Rel)
			logf("wiki: replaced %s (forced)", a.Rel)
		case man.Assets[a.Rel] == digest(have):
			// The file is an older seeded copy that nobody changed.
			if err := write(path, a.Data); err != nil {
				return res, err
			}
			next.Assets[a.Rel] = want
			res.Updated = append(res.Updated, a.Rel)
			logf("wiki: updated %s (was seeded by an older iugum)", a.Rel)
		default:
			if d, ok := man.Assets[a.Rel]; ok && d != "" {
				next.Assets[a.Rel] = d
			}
			res.Skipped = append(res.Skipped, a.Rel)
			logf("wiki: kept %s (the file in the space differs from the built-in copy; iugum changed nothing). Move it aside, or set IUGUM_WIKI_SEED=force to take the built-in copy.", a.Rel)
		}
	}
	if res.WasStale && !res.Changed() {
		logf("wiki: space seed version %d is older than %d; the built-in assets did not change", man.Version, Version)
	}
	if err := writeManifest(dir, man, next); err != nil {
		return res, err
	}
	if res.Changed() {
		logf("wiki: run \"Space: Reindex\" from the command palette once, so SilverBullet loads the new space-lua and space-style.")
	}
	return res, nil
}

type manifest struct {
	Version int               `json:"iugum_seed_version"`
	Assets  map[string]string `json:"assets"`
}

func readManifest(dir string) (manifest, error) {
	m := manifest{Assets: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	// A damaged manifest is not fatal. Treat it as no record: every present
	// file then reads as one iugum must not touch.
	var parsed manifest
	if err := json.Unmarshal(b, &parsed); err != nil {
		return m, nil
	}
	if parsed.Assets == nil {
		parsed.Assets = map[string]string{}
	}
	return parsed, nil
}

func writeManifest(dir string, old, next manifest) error {
	if same(old, next) {
		return nil
	}
	keys := make([]string, 0, len(next.Assets))
	for k := range next.Assets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := manifest{Version: next.Version, Assets: map[string]string{}}
	for _, k := range keys {
		ordered.Assets[k] = next.Assets[k]
	}
	b, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return write(filepath.Join(dir, ManifestName), b)
}

func same(a, b manifest) bool {
	if a.Version != b.Version || len(a.Assets) != len(b.Assets) {
		return false
	}
	for k, v := range a.Assets {
		if b.Assets[k] != v {
			return false
		}
	}
	return true
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
