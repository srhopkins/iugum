#!/usr/bin/env bash
# Build the pinned fork UI plus Iugum-owned activity and board controls.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/iugum-beadview-build.XXXXXX")"
trap 'rm -rf "$BUILD_DIR"' EXIT
node "$ROOT/web/beadview/prepare.mjs" "$BUILD_DIR"
# prepare may reuse preinstalled dependencies; install the pinned set in this
# disposable directory instead when those dependencies are unavailable/older.
if [ -L "$BUILD_DIR/node_modules" ]; then unlink "$BUILD_DIR/node_modules"; fi
(
  cd "$BUILD_DIR"
  npm ci --ignore-scripts --cache "${IUGUM_NPM_CACHE:-/private/tmp/iugum-npm-cache}"
  npm test
  npm run build
)
node "$ROOT/web/beadview/sync-assets.mjs" "$BUILD_DIR/dist"
echo 'Iugum beadview UI built. Rebuild iugum to embed it.'
