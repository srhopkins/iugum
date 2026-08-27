#!/bin/sh
# Daily `bd` is iugum. Homebrew Beads stays at /opt/homebrew/bin/bd.
exec "${IUGUM:-$HOME/.local/bin/iugum}" beads "$@"
