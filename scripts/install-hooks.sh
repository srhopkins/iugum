#!/usr/bin/env bash
# Point this clone at .githooks (public-audit + beads). Git does not install hooks on clone.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
chmod +x "$root/scripts/public-audit.sh" "$root/scripts/public-audit-agent.sh" "$root/.githooks"/* || true
git -C "$root" config core.hooksPath .githooks
echo "hooksPath=$(git -C "$root" config --get core.hooksPath)"
echo "public-audit: $root/scripts/public-audit.sh --staged"
