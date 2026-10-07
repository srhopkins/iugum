#!/bin/bash
# Run every script in the mounted install.d, in name order. Never fail start.
DIR="${INSTALL_D:-/opt/agentbox/install.d}"
[ -d "$DIR" ] || { echo "[install.d] no $DIR, skipping"; exit 0; }
for f in "$DIR"/*; do
  [ -f "$f" ] || continue
  case "$f" in *.md) continue ;; esac
  if [ -x "$f" ]; then run=("$f"); elif [[ "$f" == *.sh ]]; then run=(bash "$f"); else echo "[install.d] skipped (not executable): $f"; continue; fi
  echo "[install.d] running $f"
  if "${run[@]}"; then :; else echo "[install.d] FAILED: $f (continuing)" >&2; fi
done
exit 0
