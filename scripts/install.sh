#!/usr/bin/env bash
# Build the current tree and put iugum on PATH. Does not overwrite config.
#
#   scripts/install.sh            CGO build (embedded Dolt works). Default.
#   scripts/install.sh --static   CGO_ENABLED=0 static build.
#
# IUGUM_INSTALL_DIR overrides the install directory (default: $HOME/.local/bin).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

scripts/build.sh "${1:---cgo}"

INSTALL_DIR="${IUGUM_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"

TARGET="$INSTALL_DIR/iugum"

# The legacy $HOME/bin mirror is only for the real, default install: a caller
# setting IUGUM_INSTALL_DIR is explicitly asking for an isolated/throwaway
# prefix (a DoD eval, a test), and that request must not also land a copy on
# a real, unrelated PATH entry.
MIRROR_BIN=0
if [ -z "${IUGUM_INSTALL_DIR:-}" ]; then
  MIRROR_BIN=1
  mkdir -p "$HOME/bin"
fi

# Capture what was there before we overwrite it, so we can tell the caller
# whether the thing they just replaced was already older than this build
# (drifted silently, same failure mode this bead exists to catch) rather than
# just "different".
prev_full=""
if [ -x "$TARGET" ]; then
  prev_full="$("$TARGET" version 2>/dev/null || echo "")"
fi

# Overwriting a running binary's file in place (cp onto an existing inode) has
# SIGKILLed it on macOS until the stale file was removed and a fresh one
# copied in and re-signed. rm-then-cp always gives the new file a fresh
# inode, so a process still holding the old one keeps running against it
# instead of racing a half-written file.
rm -f "$TARGET"
cp "$ROOT/iugum" "$TARGET"
chmod +x "$TARGET"

if [ "$MIRROR_BIN" = "1" ]; then
  # Same reasoning for the ~/bin copy some shells still have on PATH.
  rm -f "$HOME/bin/iugum"
  cp "$ROOT/iugum" "$HOME/bin/iugum"
  chmod +x "$HOME/bin/iugum"
fi

if [ "$(uname -s)" = "Darwin" ]; then
  codesign -s - -f "$TARGET" >/dev/null 2>&1 || true
  if [ "$MIRROR_BIN" = "1" ]; then
    codesign -s - -f "$HOME/bin/iugum" >/dev/null 2>&1 || true
  fi
fi

install -m 0755 "$ROOT/scripts/bd-shim.sh" "$INSTALL_DIR/bd"

echo "installed $TARGET"
echo "bd -> $INSTALL_DIR/iugum beads"

# version_line extracts just the "version: <commit>" line from `iugum version`
# output, so the built-vs-installed comparison below is a plain string compare
# instead of parsing all four fields.
version_line() {
  "$1" version 2>/dev/null | sed -n '1p'
}

built_full="$("$ROOT/iugum" version 2>/dev/null || echo "unknown")"
installed_full="$("$TARGET" version 2>/dev/null || echo "unknown")"
built_ver="$(version_line "$ROOT/iugum")"
installed_ver="$(version_line "$TARGET")"

echo "built: $(echo "$built_full" | tr '\n' ' ')"
echo "installed: $(echo "$installed_full" | tr '\n' ' ')"

if [ "$built_ver" != "$installed_ver" ]; then
  echo "WARNING: installed version does not match the build just produced ('$installed_ver' vs '$built_ver')." >&2
  echo "WARNING: run this script again, or check that $TARGET is really $ROOT/iugum." >&2
fi

# build_date_of extracts "build date: ..." from a captured `iugum version`
# blob, so we can tell an older previous install from a merely-different one.
build_date_of() {
  echo "$1" | sed -n 's/^build date: //p'
}

if [ -n "$prev_full" ]; then
  prev_date="$(build_date_of "$prev_full")"
  built_date="$(build_date_of "$built_full")"
  if [ -n "$prev_date" ] && [ -n "$built_date" ] && [ "$prev_date" \< "$built_date" ]; then
    echo "WARNING: the binary this replaced was older (built $prev_date, this build is $built_date)." >&2
    echo "WARNING: it had drifted silently from the repo with no way to tell until now." >&2
  fi
fi
