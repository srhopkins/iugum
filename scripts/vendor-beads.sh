#!/usr/bin/env bash
# Re-vendor beads/ from an upstream module version and re-apply the iugum patches.
#
#   scripts/vendor-beads.sh <module-version>
#   scripts/vendor-beads.sh v1.1.1-0.20260719023420-b2b153b7b834
#
# Pick the version that matches the bd binary that writes your .beads Dolt DB.
# The schema version must match. `bd version` prints the commit; the module
# version is v<next>-0.<timestamp>-<12-char commit> (go mod download resolves it
# from a bare commit hash too: scripts/vendor-beads.sh b2b153b7b834).
#
# Steps:
#   1. go mod download the version; copy the module tree over beads/ (writable).
#   2. Rename `package main` to `package bdcmd` in beads/cmd/bd/*.go.
#   3. Apply scripts/vendor/beads-patches/*.patch (Execute export, MemoryHook).
#   4. Update the beads require line in go.mod; go mod tidy; gofmt; static build check.
# Docs: docs/beads-vendor.md
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
PATCHES="$ROOT/scripts/vendor/beads-patches"
MOD="github.com/steveyegge/beads"

VER="${1:-}"
[ -n "$VER" ] || { sed -n '2,18p' "$0"; exit 2; }

die() { echo "vendor-beads: $*" >&2; exit 1; }

echo "== download $MOD@$VER"
# Drop the local replace for the download so Go asks the proxy, not ./beads.
JSON="$(cd "$(mktemp -d)" && go mod init tmp >/dev/null 2>&1 && go mod download -json "$MOD@$VER")"
DIR="$(printf '%s' "$JSON" | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",*$/\1/p')"
RESOLVED="$(printf '%s' "$JSON" | sed -n 's/^[[:space:]]*"Version": "\(.*\)",*$/\1/p')"
[ -d "$DIR" ] || die "module dir not found in: $JSON"
echo "resolved $RESOLVED at $DIR"

echo "== replace beads/"
rm -rf beads
cp -R "$DIR" beads
chmod -R u+w beads

echo "== rename package main -> package bdcmd (beads/cmd/bd/*.go)"
for f in beads/cmd/bd/*.go; do
  sed -i.bak '1,/^package main$/s/^package main$/package bdcmd/' "$f" && rm -f "$f.bak"
done
LEFT="$(grep -l '^package main$' beads/cmd/bd/*.go || true)"
[ -z "$LEFT" ] || die "package main still present: $LEFT"

echo "== apply patches"
for p in "$PATCHES"/*.patch; do
  echo "  $(basename "$p")"
  (cd beads && patch -p1 --no-backup-if-mismatch --forward < "$p") || die "patch failed: $p. Fix beads/cmd/bd by hand, then regenerate the patch (see docs/beads-vendor.md)."
done
grep -q '^func Execute() {' beads/cmd/bd/main.go || die "Execute not exported in beads/cmd/bd/main.go"
grep -q 'func SetMemoryHook' beads/cmd/bd/memory.go || die "SetMemoryHook missing in beads/cmd/bd/memory.go"

echo "== go.mod"
sed -i.bak "s#^\([[:space:]]*\)$MOD v[^ ]*#\1$MOD $RESOLVED#" go.mod && rm -f go.mod.bak
grep -q "^replace $MOD => ./beads" go.mod || die "go.mod lost the replace line"
gofmt -w beads/cmd/bd/main.go beads/cmd/bd/memory.go beads/cmd/bd/prime.go
go mod tidy

echo "== static build check"
CGO_ENABLED=0 go build -o /dev/null . || die "static build failed after vendoring"
echo "vendored $MOD $RESOLVED into beads/"
echo "next: scripts/build.sh --cgo && ./iugum beads list"
