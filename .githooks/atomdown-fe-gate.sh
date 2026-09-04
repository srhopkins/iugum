#!/usr/bin/env sh
#
# The conditional half of the Atomdown front-end gate.
#
# Called from `.githooks/pre-push` with the repo root as $1 and git's ref list
# on stdin. It decides whether this push touches anything the front-end suite
# tests, and runs the suite only if it does.
#
# WHY CONDITIONAL. The suite drives a real browser over a real document and
# takes about two minutes. A push that changes a Go file or a doc must not pay
# that, or the gate gets bypassed within a week and stops protecting anything.
# So it watches exactly three paths:
#
#   plugs/atomdown-board/**    the board panel
#   plugs/atomdown-inline/**   the inline view
#   silverbullet/client/**     the editor decoration seam the inline view needs
#
# ...plus the suite itself, because a change to a rule should be run before it
# is pushed.
#
# WHY PRE-PUSH AND NOT PRE-COMMIT. Commits are checkpoints and must stay
# instant — see the commit-discipline section in AGENTS.md. Push is the review
# gate. This is the same division the repo already uses for the public audit.

set -eu

root="${1:?usage: atomdown-fe-gate.sh <repo-root> (ref list on stdin)}"

say() { printf 'atomdown-fe-gate: %s\n' "$1" >&2; }

# --- The escape hatch --------------------------------------------------------
#
# Documented, loud, and it names itself in the output so a skipped push is
# visible in a terminal scrollback rather than silent. For a genuine emergency:
# a hotfix at 2am, or a machine with no browser.
if [ "${ATOMDOWN_FE_SKIP:-}" = "1" ]; then
  say "SKIPPED by ATOMDOWN_FE_SKIP=1 — the front-end suite did NOT run"
  say "run it before the next push: scripts/atomdown-fe-check.sh"
  exit 0
fi

# --- What is in this push ----------------------------------------------------
#
# git sends one line per ref: <local ref> <local sha> <remote ref> <remote sha>.
# A remote sha of all zeros means the remote has no such branch yet, so there is
# no range to diff; fall back to the merge base with the default branch, and to
# the single commit if even that is unknown.
zero="0000000000000000000000000000000000000000"
changed=""

while read -r _local_ref local_sha _remote_ref remote_sha; do
  # A local sha of zeros is a branch DELETION. Nothing to test.
  [ "$local_sha" = "$zero" ] && continue

  if [ "$remote_sha" = "$zero" ]; then
    base="$(git -C "$root" merge-base origin/main "$local_sha" 2>/dev/null || true)"
    [ -z "$base" ] && base="$(git -C "$root" rev-parse "$local_sha^" 2>/dev/null || true)"
    [ -z "$base" ] && base="$local_sha"
  else
    base="$remote_sha"
  fi

  files="$(git -C "$root" diff --name-only "$base" "$local_sha" 2>/dev/null || true)"
  changed="$changed
$files"
done

watched="$(printf '%s\n' "$changed" | grep -E \
  '^(plugs/atomdown-board/|plugs/atomdown-inline/|silverbullet/client/|plugs/atomdown-e2e/|scripts/atomdown-fe-check\.sh)' \
  || true)"

if [ -z "$watched" ]; then
  say "no changes to the board plug, the inline plug or the vendored client — skipping"
  exit 0
fi

say "this push touches the atomdown views:"
printf '%s\n' "$watched" | sort -u | sed 's/^/atomdown-fe-gate:   /' >&2
say "running the front-end suite (fast matrix)"

# The check script prints its own elapsed time, the failing rule number and the
# artifact directory, so this hook adds nothing but the decision to call it.
"$root/scripts/atomdown-fe-check.sh" >&2 || {
  say "PUSH BLOCKED by the atomdown front-end suite"
  say "emergency escape hatch: ATOMDOWN_FE_SKIP=1 git push"
  exit 1
}

exit 0
