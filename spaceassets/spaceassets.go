// Package spaceassets carries the atomdown wiki space assets: the two plug
// bundles and the library page that holds their header button and their CSS.
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
// The assets arrive through Set, the way embedbin takes the SilverBullet
// binary: they are build outputs under plugs/, so root package main embeds
// them.
//
// # What does not belong here
//
// Only files the atomdown views cannot work without. The bundle exists so the
// feature can never look broken, not as a place to ship conveniences.
//
// A page that changes global editor behaviour must not be bundled. A base_fs
// page can be overridden only at its own exact path, so a page that also
// exists in a space under a different name gives two copies with no way to
// turn either off. That was measured: bundling an editor-width page put two
// width buttons in the top bar of a space that had its own copy, and the two
// system:ready listeners raced over one html attribute and one clientStore
// key. The atomdown pages carry no such risk, because nothing else defines
// them.
package spaceassets

import (
	"bytes"
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
// holds the string; one built without them does not. The wiki adapter uses it
// to tell a stale binary from a current one.
const Marker = Namespace + "/Inline.md"

// Asset is one file to stage. Rel is the path inside Namespace, with forward
// slashes. Src is where the file comes from in this repository, relative to the
// repository root and with forward slashes. Src exists so a running program can
// compare the bytes it carries against the bytes on disk; an asset with no Src
// is never compared.
type Asset struct {
	Rel  string
	Data []byte
	Src  string
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

// All returns every asset, sorted by Rel.
func All() ([]Asset, error) {
	mu.Lock()
	out := make([]Asset, len(set))
	copy(out, set)
	mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

// EnvSourceRoot names the repository root to compare the carried assets
// against. It overrides the search below, for a checkout that is not above the
// working directory.
const EnvSourceRoot = "IUGUM_PLUG_SRC"

// SourceRoot returns the repository root whose plug sources this program can
// compare itself against, or "" when there is none.
//
// HOW THE ROOT IS FOUND, and why this way. The assets are //go:embed-ed, so the
// bytes are frozen at compile time and a forgotten rebuild is invisible at run
// time. To see that, the program has to find the sources again. Three ways were
// possible:
//
//   - A path recorded at build time with -ldflags -X. Rejected: the plain
//     `go build ./...` that a person actually runs records nothing, so the one
//     build most likely to be stale would carry no check at all.
//   - An environment variable only. Rejected as the sole mechanism: a check
//     that needs setup does not fire on the day it is needed. It is kept as an
//     override.
//   - A walk up from the working directory, looking for go.mod beside plugs/.
//     Chosen: it needs no build flag and no setup, it fires for every build
//     path, and it costs a few Stat calls.
//
// Absence is silent on purpose. A shipped binary runs on machines with no
// repository, and a wrong warning there is worse than no warning.
func SourceRoot() string {
	if p := os.Getenv(EnvSourceRoot); p != "" {
		if isSourceRoot(p) {
			return p
		}
		return ""
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if isSourceRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isSourceRoot reports whether dir looks like this repository: a Go module with
// a plugs directory. Both are required, so an unrelated module above the
// working directory is not mistaken for one.
func isSourceRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "plugs"))
	return err == nil && info.IsDir()
}

// Status is one carried asset measured against its source file.
type Status struct {
	Rel      string // path inside Namespace
	Src      string // repository-relative source path
	Carried  int    // bytes this program carries and will serve
	OnDisk   int    // bytes of the source file
	Found    bool   // the source file was read
	Current  bool   // the carried bytes are the source bytes
	SizeOnly bool   // the two files differ in content at the same size
}

// Compare measures every asset that names a source against the file at
// root/Src. Assets with no Src, and sources that cannot be read, come back
// Found=false and are not drift: the check is best effort.
func Compare(root string) []Status {
	assets, err := All()
	if err != nil {
		return nil
	}
	out := make([]Status, 0, len(assets))
	for _, a := range assets {
		st := Status{Rel: a.Rel, Src: a.Src, Carried: len(a.Data)}
		if a.Src == "" {
			out = append(out, st)
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.Src)))
		if err != nil {
			out = append(out, st)
			continue
		}
		st.Found = true
		st.OnDisk = len(data)
		st.Current = bytes.Equal(a.Data, data)
		st.SizeOnly = !st.Current && st.Carried == st.OnDisk
		out = append(out, st)
	}
	return out
}

// Stale returns the statuses that are read and out of date.
func Stale(sts []Status) []Status {
	out := make([]Status, 0, len(sts))
	for _, st := range sts {
		if st.Found && !st.Current {
			out = append(out, st)
		}
	}
	return out
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
