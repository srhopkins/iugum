#!/usr/bin/env bash
# Build the current tree and put iugum on PATH. Does not overwrite config.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CGO_ENABLED=0 go build -o iugum .

mkdir -p "$HOME/.local/bin" "$HOME/bin"
cp -f iugum "$HOME/.local/bin/iugum"
cp -f iugum "$HOME/bin/iugum"

install -m 0755 "$ROOT/scripts/bd-shim.sh" "$HOME/.local/bin/bd"
chmod +x "$HOME/.local/bin/iugum" "$HOME/bin/iugum"

echo "installed $HOME/.local/bin/iugum"
echo "bd -> $HOME/.local/bin/iugum beads"
