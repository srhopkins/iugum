#!/usr/bin/env bash
# DoD gate for iugum-wuz — "iugum beads init fails: no beads database found".
source "$(dirname "$0")/_lib.sh"; cd "$ROOT"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "== build"
check "cgo build" scripts/build.sh --cgo
check "go vet" go vet ./...
check "static build compiles" env CGO_ENABLED=0 go build -o "$TMP/iugum-static" .
IUG="$ROOT/iugum"
echo "== no-DB commands outside any workspace"
cd "$TMP"
check "beads version" "$IUG" beads version
check "beads where" "$IUG" beads where
check "beads prime" "$IUG" beads prime
check "beads quickstart --help" "$IUG" beads quickstart --help
check "beads setup --help" "$IUG" beads setup --help
check "beads --help" "$IUG" beads --help
echo "== init in a fresh repo"
mkdir -p "$TMP/fresh" && cd "$TMP/fresh" && git init -q .
check_out "beads init" 'zz|Initialized|initialized|quickstart' "$IUG" beads init --prefix zz --non-interactive
check ".beads created" test -d .beads
check "beads create" "$IUG" beads create "gate probe" -p 2
check_out "beads list shows zz-1" 'zz-1' "$IUG" beads list
check_out "beads ready shows zz-1" 'zz-1' "$IUG" beads ready
echo "== memory hook still routes"
check "remember" "$IUG" beads remember --key dod-probe-iugum-wuz "probe"
check_out "memories" 'probe' "$IUG" beads memories dod-probe
"$IUG" beads forget dod-probe-iugum-wuz >/dev/null 2>&1 || true
echo "== regression test + patch reproducibility"
cd "$ROOT"
check "beadsadapt tests" env CGO_ENABLED=0 go test ./adapter/tracker/... ./
check_out "bead notes state root cause" 'BD_NAME|root cause|isSubcommand' /opt/homebrew/bin/bd show iugum-wuz
if git diff --quiet HEAD -- beads/ 2>/dev/null && ! git status --short beads/ | grep -q .; then pass "beads/ unchanged (fix in iugum code)"; else
  check_out "vendor script reproduces tree" 'identical|no diff|OK' bash -c 'cp -R beads "$0/beads-before" && scripts/vendor-beads.sh "$(grep -m1 "steveyegge/beads " go.mod | awk "{print \$2}")" >/dev/null 2>&1; if diff -rq beads "$0/beads-before" >/dev/null; then echo identical; else echo DIFF; fi' "$TMP"
fi
echo "== yq2 still passes"
check "yq2 gate" bash scripts/dod/iugum-yq2.sh
finish
