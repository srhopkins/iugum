#!/usr/bin/env bash
# Deterministic public-repo audit. Hook and agents both run this.
# Exit 1 on secrets. Warn (exit 0) on personal or local strings.
set -euo pipefail

root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  echo "public-audit: not a git repo" >&2
  exit 2
}
cd "$root"

mode="staged"
agent="auto"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --staged) mode="staged"; shift ;;
    --all) mode="all"; shift ;;
    --agent) agent="on"; shift ;;
    --no-agent) agent="off"; shift ;;
    -h|--help)
      echo "Usage: scripts/public-audit.sh [--staged|--all] [--agent|--no-agent]"
      exit 0
      ;;
    *) echo "public-audit: unknown flag $1" >&2; exit 2 ;;
  esac
done

if [[ -n "${IUGUM_AUDIT_AGENT:-}" ]]; then
  case "$IUGUM_AUDIT_AGENT" in
    0|false|off) agent="off" ;;
    1|true|on) agent="on" ;;
  esac
fi

if ! command -v gitleaks >/dev/null 2>&1; then
  echo "public-audit: gitleaks is not on PATH. Install: brew install gitleaks" >&2
  exit 1
fi

cfg="$root/.gitleaks.toml"
warn_file="$root/scripts/public-warn.patterns"
report="$(mktemp)"
trap 'rm -f "$report"' EXIT

echo "public-audit: gitleaks (block on secrets)"
set +e
if [[ "$mode" == "staged" ]]; then
  gitleaks protect --staged --no-banner --redact --config "$cfg" --report-path "$report" --report-format json --verbose
  gl=$?
else
  gitleaks dir --no-banner --redact --config "$cfg" --report-path "$report" --report-format json --verbose
  gl=$?
fi
set -e
if [[ $gl -ne 0 ]]; then
  echo "public-audit: BLOCK secrets (gitleaks exit $gl)" >&2
  exit 1
fi

warns=0
if [[ -f "$warn_file" ]] && command -v rg >/dev/null 2>&1; then
  echo "public-audit: personal-content patterns (warn)"
  pat_tmp="$(mktemp)"
  grep -v '^#' "$warn_file" | grep -v '^[[:space:]]*$' >"$pat_tmp" || true
  if [[ -s "$pat_tmp" ]]; then
    set +e
    if [[ "$mode" == "staged" ]]; then
      git diff --cached -U0 -- . \
        ':(exclude)beads' ':(exclude)silverbullet' ':(exclude)scripts/public-warn.patterns' \
        | rg -n -f "$pat_tmp"
    else
      rg -n -f "$pat_tmp" --glob '!beads/**' --glob '!silverbullet/**' --glob '!.git/**' --glob '!scripts/public-warn.patterns' .
    fi
    wr=$?
    set -e
    if [[ $wr -eq 0 ]]; then
      warns=1
      echo "public-audit: WARN personal or local strings. Do not put those in a public commit." >&2
    fi
  fi
  rm -f "$pat_tmp"
elif [[ -f "$warn_file" ]]; then
  echo "public-audit: rg not on PATH; skip personal-content warn list" >&2
fi

if [[ "$agent" == "on" ]] || { [[ "$agent" == "auto" ]] && [[ $warns -eq 1 ]]; }; then
  if [[ -x "$root/scripts/public-audit-agent.sh" ]]; then
    echo "public-audit: residual agent pass"
    "$root/scripts/public-audit-agent.sh" "$mode" || exit 1
  fi
fi

echo "public-audit: ok"
exit 0
