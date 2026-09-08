#!/usr/bin/env bash
# Build the embedded SilverBullet binary with iugum's space assets in it.
#
#   scripts/build-wiki-blob.sh [sb-source-dir]
#
# The default source directory is ./silverbullet, the vendored subtree.
#
# Why this script exists
# ---------------------
# The atomdown plugs and the library page that carries their CSS used to be
# hand-copied into every space. That broke four times. They now go into
# SilverBullet's client_bundle/base_fs, the read-only underlay that rust-embed
# compiles into the SilverBullet binary. Every space then reads them, no space
# holds a copy, and no space can delete them.
#
# base_fs is a build output, so the assets have to be staged between the two
# halves of the SilverBullet build:
#
#   npm run build      writes client_bundle/{client,base_fs}
#   iugum stage-wiki-assets   adds client_bundle/base_fs/Library/Atomdown
#   cargo build        compiles base_fs into the binary
#
# `make build-rs` runs the first and third steps together, so this script runs
# them separately instead. Nothing in the vendored tree is edited: base_fs is
# gitignored there, and the staged files are added, not merged into an
# upstream file.
#
# TWO THINGS THIS SCRIPT NOW DOES FOR YOU, because both cost real time before:
#
#   1. IT BUILDS A FRESH STAGER EVERY TIME. The space assets are //go:embed-ed
#      into package main, so `iugum stage-wiki-assets` stages the plug files
#      that were in the tree when THAT binary was built. Reusing an existing
#      ./iugum silently staged a plug from hours earlier, and the wiki then
#      served the old one with nothing anywhere saying so. The front-end
#      suite's own runner already learned this; see
#      scripts/atomdown-fe-check.sh.
#   2. IT RE-BUILDS iugum AT THE END, so the new blob is embedded. The
#      sequence used to be build.sh, build-wiki-blob.sh, build.sh again, with
#      the second build.sh left to the reader - and a forgotten third step
#      looks exactly like a plug change that did not work.
#
# NODE IS PINNED. Node 18 cannot build the client, and the failure is not
# obvious. If `node` on PATH is older than 20, this script looks for a newer
# one under nvm and uses that, or stops and says which versions it found.
#
# Needs npm, cargo and make on PATH. The compile takes minutes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SRC="${1:-$ROOT/silverbullet}"
case "${1:-}" in
  -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
esac

die() { echo "build-wiki-blob.sh: $*" >&2; exit 1; }

[ -f "$SRC/Makefile" ] || die "$SRC is not a SilverBullet source tree (no Makefile)"
command -v npm   >/dev/null 2>&1 || die "npm not on PATH"
command -v cargo >/dev/null 2>&1 || die "cargo not on PATH. Install Rust: https://rustup.rs"

# --- Pin node ---------------------------------------------------------------
#
# Node 18 cannot build the client and cannot load a plug in the test harness.
# The error it gives says neither of those things, so this checks instead.
NODE_MIN=20
node_major() {
  command -v node >/dev/null 2>&1 || { echo 0; return; }
  node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0
}
if [ "$(node_major)" -lt "$NODE_MIN" ]; then
  found=""
  for candidate in "$HOME"/.nvm/versions/node/v*/bin; do
    [ -x "$candidate/node" ] || continue
    ver="$("$candidate/node" -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)"
    found="$found v$("$candidate/node" -p 'process.versions.node' 2>/dev/null)"
    if [ "$ver" -ge "$NODE_MIN" ]; then
      echo "==> node $(node -v 2>/dev/null || echo none) is too old; using $candidate"
      PATH="$candidate:$PATH"
      export PATH
      break
    fi
  done
  if [ "$(node_major)" -lt "$NODE_MIN" ]; then
    die "node $NODE_MIN or later is required to build the client. On PATH: $(node -v 2>/dev/null || echo none).${found:+ Found under nvm:$found}"
  fi
fi
echo "==> node $(node -v)"

# ALWAYS A FRESH STAGER, never ./iugum. The space assets are //go:embed-ed into
# package main, so an existing ./iugum stages the plug files that were in the
# tree when IT was built. That is the stale-plug trap: one pass through this
# script shipped an hours-old plug and nothing said so.
echo "==> building a stager, so the staged assets are the ones in this tree"
CGO_ENABLED=0 go build -o "$ROOT/iugum-stager" .
STAGER="$ROOT/iugum-stager"
trap 'rm -f "$ROOT/iugum-stager"' EXIT

echo "==> npm install (skipped when node_modules is present)"
[ -d "$SRC/node_modules" ] || (cd "$SRC" && npm install)

echo "==> npm run build  (client bundle and base_fs)"
(cd "$SRC" && npm run build)

echo "==> stage the iugum space assets into base_fs"
"$STAGER" stage-wiki-assets "$SRC"

echo "==> cargo build --release -p silverbullet"
(cd "$SRC" && cargo build --release -p silverbullet)

ART="$SRC/target/release/silverbullet"
[ -x "$ART" ] || die "the build made no $ART"

# install, NOT cp, and into wikiblob/ (iugum-ef0). main.go embeds the wikiblob
# DIRECTORY rather than this file, so a checkout that has never built a server
# still compiles. install sets the mode explicitly: cp onto an existing file
# keeps the DESTINATION's mode, which once shipped a non-executable server.
echo "==> install as the //go:embed target"
install -m 0755 "$ART" "$ROOT/wikiblob/silverbullet"

# THE SECOND PASS, done here rather than left to the reader. The blob above is
# what `//go:embed` picks up, so iugum has to be rebuilt for the new blob to be
# inside it - and a forgotten step looks exactly like a plug change that did
# not work.
echo "==> rebuilding iugum so the new blob is embedded"
"$ROOT/scripts/build.sh" "${IUGUM_BUILD_MODE:---cgo}"

echo
echo "done. wikiblob/silverbullet carries the space assets, and ./iugum"
echo "embeds that blob. No second scripts/build.sh is needed."
