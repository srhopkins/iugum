#!/usr/bin/env bash
# Residual pass after gitleaks + regex. Cannot waive a secret block.
# Skip when cursor-agent is missing. Set IUGUM_AUDIT_AGENT=0 to disable.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
mode="${1:-staged}"

if ! command -v cursor-agent >/dev/null 2>&1; then
  echo "public-audit-agent: cursor-agent not on PATH; skip residual pass"
  exit 0
fi

diff_txt="$(mktemp)"
trap 'rm -f "$diff_txt"' EXIT
if [[ "$mode" == "staged" ]]; then
  git diff --cached -- . ':(exclude)beads' ':(exclude)silverbullet' >"$diff_txt"
else
  git diff -- . ':(exclude)beads' ':(exclude)silverbullet' >"$diff_txt"
fi

# Empty diff: nothing for the agent to judge.
if [[ ! -s "$diff_txt" ]]; then
  echo "public-audit-agent: no diff; skip"
  exit 0
fi

prompt="$(cat <<EOF
You audit a public git repo (iugum). Deterministic tools already ran.
gitleaks found no secrets. A regex list may have printed WARN lines.

Read the staged or working diff. Do not repeat the tool findings.

Exit rules:
- Print BLOCK and exit 1 only if you find a secret, password, token, or private key the tools missed.
- Print WARN for personal names, home paths, private emails, LAN IPs, or local-only comments that should not be public.
- Print OK and exit 0 if the diff is safe to publish.

Do not print the secret value. Name the file and the kind of finding.
EOF
)"

set +e
cursor-agent -p "$prompt" --output-format text --trust <"$diff_txt"
ac=$?
set -e
if [[ $ac -ne 0 ]]; then
  echo "public-audit-agent: BLOCK residual finding (exit $ac)" >&2
  exit 1
fi
exit 0
