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

# The staging step lives in the iugum program, because the assets are embedded
# in it. Build a throwaway iugum first if there is none: the static build needs
# no ICU and is enough to stage files.
STAGER="$ROOT/iugum"
if [ ! -x "$STAGER" ]; then
  echo "==> no ./iugum yet; building a static one to stage the assets"
  CGO_ENABLED=0 go build -o "$ROOT/iugum-stager" .
  STAGER="$ROOT/iugum-stager"
  trap 'rm -f "$ROOT/iugum-stager"' EXIT
fi

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

echo "==> install as the //go:embed target"
cp "$ART" "$ROOT/silverbullet/silverbullet"

echo
echo "done. silverbullet/silverbullet now carries the space assets."
echo "Rebuild iugum so the new blob is embedded:  scripts/build.sh"
