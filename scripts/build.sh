#!/usr/bin/env bash
# Build ./iugum in one of two modes.
#
#   scripts/build.sh [--cgo]     CGO_ENABLED=1. Embedded Dolt works (iugum beads). Default.
#   scripts/build.sh --static    CGO_ENABLED=0. Static program. Beads needs server mode.
#
# The CGO build links ICU (Unicode regex library) for the Beads Dolt store.
# ICU is a deliberate, temporary deviation from the CGO_ENABLED=0 rule.
# See NORTHSTARS.md star 1 and CONTRIBUTING.md.
#
# ICU install:
#   macOS   brew install icu4c          (keg-only; this script sets the include/lib paths)
#   Debian  sudo apt-get install libicu-dev g++ pkg-config
#   Fedora  sudo dnf install libicu-devel gcc-c++ pkgconfig
#   Alpine  apk add icu-dev g++ pkgconfig
#
# Override the ICU prefix with ICU_PREFIX=/path (must hold include/unicode/regex.h and lib/).
#
# Bare `go vet ./...` and `go test` (CGO on) also need the ICU paths. One-time setup on macOS:
#   go env -w CGO_CPPFLAGS="-I$(brew --prefix icu4c)/include" CGO_LDFLAGS="-L$(brew --prefix icu4c)/lib"
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MODE="cgo"
case "${1:-}" in
  ""|--cgo) MODE="cgo" ;;
  --static) MODE="static" ;;
  -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
  *) echo "build.sh: unknown flag '$1' (use --cgo or --static)" >&2; exit 2 ;;
esac

die() { echo "build.sh: $*" >&2; exit 1; }

# version_ldflags builds the -X assignments that stamp main.buildCommit,
# main.buildDate, and main.buildCGO into the binary, so `iugum version` can
# report what it was built from without relying on runtime/debug's vcs.*
# settings (those are only populated with -buildvcs=true and a git tree; the
# tower builds inside a container with -buildvcs=false and no .git).
#
# git rev-parse fails outside a git tree (or with none present); that is not
# a build failure, just a build with no vcs.* fallback either, so version()
# falls back to "unknown".
version_ldflags() {
  local cgo_flag="$1" commit="" dirty="" date
  if command -v git >/dev/null 2>&1 && git rev-parse --short=12 HEAD >/dev/null 2>&1; then
    commit="$(git rev-parse --short=12 HEAD)"
    if ! git diff --quiet --ignore-submodules HEAD -- 2>/dev/null; then
      dirty="-dirty"
    fi
  fi
  # No space in the stamped value: ldflags -X splits its argument string on
  # whitespace, so a literal "YYYY-MM-DD HH:MM" here breaks the link with a
  # stray unparsed token. Stamp with a T separator instead; version.go turns
  # it back into "YYYY-MM-DD HH:MM" when it prints.
  date="$(date -u +%Y-%m-%dT%H:%M)"
  echo "-X main.buildCommit=${commit}${dirty} -X main.buildDate=${date} -X main.buildCGO=${cgo_flag}"
}

icu_env_darwin() {
  local prefix="${ICU_PREFIX:-}"
  if [ -z "$prefix" ] && command -v brew >/dev/null 2>&1; then
    prefix="$(brew --prefix icu4c 2>/dev/null || true)"
  fi
  if [ -z "$prefix" ] || [ ! -f "$prefix/include/unicode/regex.h" ]; then
    die "ICU headers not found (unicode/regex.h). Run: brew install icu4c   (or set ICU_PREFIX=/path)"
  fi
  export CGO_CPPFLAGS="${CGO_CPPFLAGS:-} -I$prefix/include"
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L$prefix/lib"
  echo "icu: $prefix"
}

icu_env_linux() {
  local prefix="${ICU_PREFIX:-}"
  if [ -n "$prefix" ]; then
    [ -f "$prefix/include/unicode/regex.h" ] || die "ICU_PREFIX=$prefix has no include/unicode/regex.h"
    export CGO_CPPFLAGS="${CGO_CPPFLAGS:-} -I$prefix/include"
    export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L$prefix/lib"
    echo "icu: $prefix"
    return
  fi
  if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists icu-uc icu-i18n 2>/dev/null; then
    export CGO_CPPFLAGS="${CGO_CPPFLAGS:-} $(pkg-config --cflags icu-uc icu-i18n)"
    export CGO_LDFLAGS="${CGO_LDFLAGS:-} $(pkg-config --libs-only-L icu-uc icu-i18n)"
    echo "icu: pkg-config $(pkg-config --modversion icu-i18n)"
    return
  fi
  if [ -f /usr/include/unicode/regex.h ]; then
    echo "icu: /usr/include"
    return
  fi
  die "ICU headers not found (unicode/regex.h). Install: apt-get install libicu-dev g++ pkg-config   (Fedora: libicu-devel gcc-c++; Alpine: icu-dev g++)"
}

if [ "$MODE" = "cgo" ]; then
  command -v cc >/dev/null 2>&1 || die "no C compiler. macOS: xcode-select --install. Linux: apt-get install g++"
  case "$(uname -s)" in
    Darwin) icu_env_darwin ;;
    Linux)  icu_env_linux ;;
    *) echo "build.sh: unknown OS $(uname -s); set CGO_CPPFLAGS/CGO_LDFLAGS for ICU yourself" >&2 ;;
  esac
  echo "mode: cgo (CGO_ENABLED=1, embedded Dolt on)"
  CGO_ENABLED=1 go build -ldflags "$(version_ldflags on)" -o iugum .
else
  echo "mode: static (CGO_ENABLED=0, embedded Dolt off; beads needs server mode)"
  CGO_ENABLED=0 go build -ldflags "$(version_ldflags off)" -o iugum .
fi

SIZE="$(du -h iugum | cut -f1)"
echo "built ./iugum ($SIZE)"
