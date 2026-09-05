// Package spaceassets carries the wiki space assets that this program owns:
// the two atomdown plug bundles, the atomdown library page, and the editor
// width page.
//
// The assets do not go into a space folder. They go into SilverBullet's
// client_bundle/base_fs, the read-only underlay that rust-embed compiles into
// the SilverBullet binary. A space then reads them without holding a copy:
//
//	client_bundle/base_fs/Library/Atomdown/...   →  the SilverBullet binary
//	                                            →  every space, read-only
//
// SilverBullet layers the underlay below the space folder
// (FallthroughSpacePrimitives in server-common/src/space/embed.rs), so a page
// on disk with the same name wins, and a file that exists only in the underlay
// cannot be written or deleted from the space.
//
// Two asset sources meet here. Assets with another home in this repository
// arrive through Set, the way embedbin takes the SilverBullet binary: the plug
// bundles are build outputs under plugs/, so root package main embeds them.
// Assets that exist only to be a space page live in library/ in this package.
package spaceassets

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
)

// Namespace is the base_fs directory that holds every asset. It is outside
// upstream's Library/Std tree on purpose: a subtree pull of SilverBullet
// rewrites Library/Std and can never collide here, and the name says whose
// pages these are.
const Namespace = "Library/Atomdown"

// baseFsDir is the staging target inside a SilverBullet source tree, relative
// to the tree root. bin/silverbullet/src/embed.rs embeds it, and
// bin/silverbullet/build.rs marks it rerun-if-changed, so a staged file lands
// in the next cargo build.
const baseFsDir = "client_bundle/base_fs"

// Marker is one staged path. A SilverBullet binary built with these assets
// holds the string; one built without them does not. resolveBinary uses it to
// tell a stale binary from a current one.
const Marker = Namespace + "/Inline.md"

//go:embed library
var libraryFS embed.FS

// Asset is one file to stage. Rel is the path inside Namespace, with forward
// slashes.
type Asset struct {
	Rel  string
	Data []byte
}

var (
	mu  sync.Mutex
	set []Asset
)

// Set records the assets that root package main embeds.
func Set(a []Asset) {
	mu.Lock()
	defer mu.Unlock()
	set = a
}

// All returns every asset, sorted by Rel: the ones from Set, then the pages in
// this package's library directory.
func All() ([]Asset, error) {
	mu.Lock()
	out := make([]Asset, len(set))
	copy(out, set)
	mu.Unlock()

	entries, err := libraryFS.ReadDir("library")
	if err != nil {
		return nil, fmt.Errorf("spaceassets: read library: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := libraryFS.ReadFile(path.Join("library", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("spaceassets: read library/%s: %w", e.Name(), err)
		}
		out = append(out, Asset{Rel: e.Name(), Data: data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// Stage writes every asset into the base_fs of the SilverBullet source tree at
// src. Run it after `npm run build` (which writes base_fs) and before
// `cargo build`, so the compile embeds the assets.
//
// Stage is a plain overwrite. base_fs is a build output, gitignored by the
// vendored tree, so there is nothing in it to preserve.
func Stage(src string) ([]string, error) {
	assets, err := All()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(src, filepath.FromSlash(baseFsDir))
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("spaceassets: %s holds no %s; run `npm run build` in that tree first: %w", src, baseFsDir, err)
	}
	written := make([]string, 0, len(assets))
	for _, a := range assets {
		rel := path.Join(Namespace, a.Rel)
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(dst, a.Data, 0o644); err != nil {
			return written, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// BinaryHasAssets reports whether the SilverBullet binary at path was built
// with these assets, by looking for Marker in its bytes. rust-embed keeps every
// embedded path as a string literal, so the marker is present exactly when the
// namespace was staged before the compile.
//
// A read error reports true, not false: a check that cannot run must not print
// a warning about a binary it never saw.
func BinaryHasAssets(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	found, err := containsMarker(f, []byte(Marker))
	if err != nil {
		return true
	}
	return found
}

// containsMarker streams r and reports whether needle appears. The window
// carries the last len(needle)-1 bytes forward, so a marker that straddles two
// reads is still found.
func containsMarker(r io.Reader, needle []byte) (bool, error) {
	const chunk = 1 << 20
	overlap := len(needle) - 1
	buf := make([]byte, chunk+overlap)
	held := 0
	for {
		n, err := r.Read(buf[held:])
		if n > 0 {
			held += n
			if bytes.Contains(buf[:held], needle) {
				return true, nil
			}
			if held > overlap {
				copy(buf, buf[held-overlap:held])
				held = overlap
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
