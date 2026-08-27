#!/usr/bin/env bash
# DoD gate for iugum-yq2 — "CGO build: embedded Dolt works in iugum beads".
source "$(dirname "$0")/_lib.sh"; cd "$ROOT"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "== build script"
check "scripts/build.sh exists" test -x scripts/build.sh
check_out "build.sh documents --cgo/--static" '(--cgo|--static)' cat scripts/build.sh
check_out "install.sh uses build.sh" 'build\.sh' cat scripts/install.sh
check "static build (--static)" scripts/build.sh --static
check "static binary runs --help" ./iugum --help
check "cgo build (--cgo)" scripts/build.sh --cgo
check "cgo binary runs --help" ./iugum --help
echo "== embedded dolt through iugum beads"
OUT=$(./iugum beads list 2>&1); RC=$?
if printf '%s' "$OUT" | grep -q 'requires a CGO build'; then fail "still: requires a CGO build"; else pass "no CGO error"; fi
if [ $RC -eq 0 ] && printf '%s' "$OUT" | grep -q 'iugum-'; then pass "beads list prints issues"; else fail "beads list rc=$RC"; printf '%s\n' "$OUT" | head -10 | sed 's/^/       | /' >&2; fi
check_out "beads ready works" 'iugum-|Ready' ./iugum beads ready
check_out "beads show this bead" 'CGO build' ./iugum beads show iugum-yq2
check_out "beads remember via sqlite hook" 'dod-probe|remember|ok|Stored|saved' ./iugum beads remember --key dod-probe-yq2 "dod probe"
check_out "beads memories reads it back" 'dod probe' ./iugum beads memories dod-probe
./iugum beads forget dod-probe-yq2 >/dev/null 2>&1 || true
echo "== portable build + tests"
check "CGO_ENABLED=0 still compiles" env CGO_ENABLED=0 go build -o "$TMP/iugum-static" .
check "go vet" go vet ./...
check "beadsadapt tests" env CGO_ENABLED=0 go test ./adapter/tracker/...
check "adapter+app+config tests (static)" env CGO_ENABLED=0 go test ./adapter/... ./app/... ./config/...
echo "== docs"
check_out "NORTHSTARS deviation note" '(CGO|CGo).*(deviation|temporary|replace)' cat NORTHSTARS.md
check_out "CONTRIBUTING permits CGO for Dolt only" '(CGO|CGo).*(Dolt|dolt)' cat CONTRIBUTING.md
check_out "AGENTS.md shows cgo build" 'CGO_ENABLED=1|build\.sh --cgo' cat AGENTS.md
check_out "README build shows both" 'CGO_ENABLED=1|build\.sh' cat README.md
check_out "docs list linux packages" 'libicu-dev' cat scripts/build.sh README.md docs/*.md 2>/dev/null
echo "== shim + esu"
check_out "bd shim shape unchanged" 'exec .*iugum.* beads "\$@"' cat scripts/bd-shim.sh
check_out "iugum-esu closed" 'CLOSED|closed' /opt/homebrew/bin/bd show iugum-esu
finish
