#!/usr/bin/env bash
#
# The Atomdown front-end suite: the one documented command.
#
#   scripts/atomdown-fe-check.sh            # fast subset, the pre-push gate
#   scripts/atomdown-fe-check.sh --full     # the whole 16-cell matrix
#   scripts/atomdown-fe-check.sh --rule 1   # one rule only
#   scripts/atomdown-fe-check.sh --probe    # print what the views render
#
# A board or inline change is not done until this passes. It is also the
# Definition-of-Done gate for a subagent working on either plug: run it, and
# paste the elapsed line.
#
# WHY THIS EXISTS RATHER THAN A MAKE TARGET. It has to do three things a target
# would not: decide whether the server binary needs rebuilding, print the
# elapsed time so the cost is visible, and name the failing rule and the
# artifact path in its own output. See docs/atomdown-fe-suite.md.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SUITE="$ROOT/plugs/atomdown-e2e"
SB="$ROOT/silverbullet"
BIN="$SB/target/release/silverbullet"
PW="$SB/node_modules/.bin/playwright"
ARTIFACTS="${ATOMDOWN_FE_ARTIFACTS:-$ROOT/scratchpad/atomdown-fe-out}"

FULL=0
PROJECT="atomdown"
GREP=""
PASSTHRU=()

while [ $# -gt 0 ]; do
  case "$1" in
    --full) FULL=1; shift ;;
    --probe) PROJECT="probe"; shift ;;
    --defects) PROJECT="defects"; shift ;;
    --rule) GREP="$2"; shift 2 ;;
    -h|--help) sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) PASSTHRU+=("$1"); shift ;;
  esac
done

start=$(date +%s)

say() { printf 'atomdown-fe: %s\n' "$1"; }

# --- The client build, treated as a cache ------------------------------------
#
# The suite drives `silverbullet/target/release/silverbullet`, which carries
# iugum's editor decoration seam and embeds the client bundle through
# rust-embed. Building it takes about 90 seconds, so it is rebuilt only when it
# is missing or older than the client sources it embeds. That is the difference
# between a gate people keep and a gate people bypass.
#
# THE PLUGS ARE PART OF THE BINARY, so they are part of the cache key. Both
# plug bundles and the library page that carries their CSS are compiled into
# the server's read-only underlay (`spaceassets`, `docs/wiki-space-assets.md`),
# and the suite seeds no copy into its space — a copy is a SECOND plug, not an
# override, and two instances of the same plug undo each other. So a plug change
# has to reach the binary before the suite can see it.
needs_build=0
if [ ! -x "$BIN" ]; then
  needs_build=1
  say "no server binary at target/release/silverbullet"
else
  for src in "$SB/client" "$ROOT/plugs/atomdown-board" "$ROOT/plugs/atomdown-inline"; do
    if [ -n "$(find "$src" -newer "$BIN" -type f -name '*.ts' -print -quit 2>/dev/null)$(find "$src" -newer "$BIN" -type f -name '*.js' -print -quit 2>/dev/null)$(find "$src" -newer "$BIN" -type f -name '*.md' -print -quit 2>/dev/null)" ]; then
      needs_build=1
      say "$(basename "$src") is newer than the built binary"
    fi
  done
fi

if [ "$needs_build" = 1 ]; then
  say "building the release server (about 90s, cached after this)"
  build_start=$(date +%s)
  # The staging step goes BETWEEN the two halves of the build: npm writes
  # client_bundle/base_fs, iugum adds Library/Atomdown to it, cargo compiles it
  # in. Same sequence as scripts/build-wiki-blob.sh, without installing the
  # result as the //go:embed blob.
  ( cd "$SB" && npm run build >/dev/null )
  stager="$ROOT/iugum"
  if [ ! -x "$stager" ]; then
    say "no ./iugum to stage the space assets; building a static one"
    CGO_ENABLED=0 go build -o "$ROOT/iugum-fe-stager" "$ROOT"
    stager="$ROOT/iugum-fe-stager"
  fi
  "$stager" stage-wiki-assets "$SB"
  [ -x "$ROOT/iugum-fe-stager" ] && rm -f "$ROOT/iugum-fe-stager"
  ( cd "$SB" && cargo build --release -p silverbullet )
  say "build took $(( $(date +%s) - build_start ))s"
else
  say "reusing the cached server binary ($(date -r "$BIN" '+%Y-%m-%d %H:%M'))"
fi

if [ ! -x "$PW" ]; then
  say "ERROR: no Playwright at silverbullet/node_modules/.bin/playwright"
  say "       run: cd silverbullet && npm install"
  exit 1
fi

# --- The matrix ---------------------------------------------------------------
if [ "$FULL" = 1 ]; then
  export ATOMDOWN_FE_FULL=1
  say "matrix: FULL — 4 widths x 2 themes x 2 densities = 16 cells"
else
  say "matrix: FAST — 4 cells, one per axis value (--full for all 16)"
fi
export ATOMDOWN_FE_ARTIFACTS="$ARTIFACTS"

args=(test --config "$SUITE/playwright.config.ts" --project="$PROJECT")
# Playwright takes a filename filter positionally.
[ -n "$GREP" ] && args+=("$GREP-")
[ ${#PASSTHRU[@]} -gt 0 ] && args+=("${PASSTHRU[@]}")

set +e
( cd "$SUITE" && "$PW" "${args[@]}" )
status=$?
set -e

elapsed=$(( $(date +%s) - start ))

if [ "$status" -ne 0 ]; then
  say "FAILED in ${elapsed}s"
  say "the failing rule number, the measured numbers and a screenshot are in:"
  say "  $ARTIFACTS"
  if [ -d "$ARTIFACTS" ]; then
    # Newest first, so the run that just failed is at the top.
    ls -t "$ARTIFACTS" 2>/dev/null | head -6 | sed 's/^/atomdown-fe:    /'
  fi
  say "escape hatch, for a genuine emergency only:"
  say "  ATOMDOWN_FE_SKIP=1 git push        # records the skip in the hook output"
  exit "$status"
fi

say "PASSED in ${elapsed}s"
