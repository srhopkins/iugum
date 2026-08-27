#!/usr/bin/env bash
# Shared helpers for scripts/dod/<bead>.sh. Source this file.
# Each DoD script is the gate for one bead: exit 0 = done, anything else = keep working.
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FAILS=0
pass() { printf '  ok   %s\n' "$*"; }
fail() { printf '  FAIL %s\n' "$*" >&2; FAILS=$((FAILS+1)); }
check() { # check "label" cmd...
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then pass "$label"; else fail "$label"; fi
}
check_out() { # check_out "label" "regex" cmd...   (stdout+stderr must match)
  local label="$1" re="$2"; shift 2
  local out; out="$("$@" 2>&1)"
  if printf '%s' "$out" | grep -Eq -- "$re"; then pass "$label"; else fail "$label (want /$re/)"; printf '%s\n' "$out" | head -20 | sed 's/^/       | /' >&2; fi
}
check_fail() { # check_fail "label" cmd...   (must exit non-zero)
  local label="$1"; shift
  if "$@" >/dev/null 2>&1; then fail "$label (expected non-zero exit)"; else pass "$label"; fi
}
check_fail_out() { # check_fail_out "label" "regex" cmd...  (non-zero AND output matches)
  local label="$1" re="$2"; shift 2
  local out rc; out="$("$@" 2>&1)"; rc=$?
  if [ $rc -ne 0 ] && printf '%s' "$out" | grep -Eq -- "$re"; then pass "$label"; else fail "$label (rc=$rc want non-zero + /$re/)"; printf '%s\n' "$out" | head -20 | sed 's/^/       | /' >&2; fi
}
need() { command -v "$1" >/dev/null 2>&1 || { echo "DoD needs '$1' on PATH" >&2; exit 2; }; }
finish() {
  if [ "$FAILS" -eq 0 ]; then echo "DoD PASS: $(basename "$0")"; exit 0; fi
  echo "DoD FAIL: $FAILS check(s) failed in $(basename "$0")" >&2; exit 1
}
# Write a Casbin model+policy pair that denies one obj/act and allows the rest.
# usage: write_deny_policy DIR OBJ ACT  -> prints path to iugum.yaml that uses them
write_deny_policy() {
  local dir="$1" obj="$2" act="$3"
  mkdir -p "$dir"
  cat > "$dir/model.conf" <<'M'
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.sub == "*" || p.sub == r.sub) && (p.obj == "*" || p.obj == r.obj || keyMatch(r.obj, p.obj)) && (p.act == "*" || p.act == r.act)
M
  printf 'p, *, *, *, allow\np, *, %s, %s, deny\n' "$obj" "$act" > "$dir/policy.csv"
  cat > "$dir/iugum.yaml" <<Y
policy:
  model: $dir/model.conf
  policy: $dir/policy.csv
Y
  echo "$dir/iugum.yaml"
}
