#!/usr/bin/env bash
# Build the current tree and put iugum on PATH. Does not overwrite config.
#
#   scripts/install.sh            CGO build (embedded Dolt works). Default.
#   scripts/install.sh --static   CGO_ENABLED=0 static build.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

scripts/build.sh "${1:---cgo}"

mkdir -p "$HOME/.local/bin" "$HOME/bin"
cp -f iugum "$HOME/.local/bin/iugum"
cp -f iugum "$HOME/bin/iugum"

install -m 0755 "$ROOT/scripts/bd-shim.sh" "$HOME/.local/bin/bd"
chmod +x "$HOME/.local/bin/iugum" "$HOME/bin/iugum"

echo "installed $HOME/.local/bin/iugum"
echo "bd -> $HOME/.local/bin/iugum beads"
