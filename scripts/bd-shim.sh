#!/bin/sh
# Daily `bd`: tickets stay on Homebrew (CGO + embedded Dolt).
# remember/recall/forget/memories go through iugum SQLite.
IUGUM="${IUGUM:-$HOME/.local/bin/iugum}"
BREW_BD="${BREW_BD:-/opt/homebrew/bin/bd}"

case "$1" in
  remember|recall|forget|memories)
    exec "$IUGUM" beads "$@"
    ;;
  prime)
    "$BREW_BD" "$@"
    status=$?
    echo ""
    echo "## iugum SQLite memories"
    "$IUGUM" beads memories
    exit "$status"
    ;;
  *)
    exec "$BREW_BD" "$@"
    ;;
esac
