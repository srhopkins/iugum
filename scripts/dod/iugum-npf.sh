#!/usr/bin/env bash
# DoD gate for iugum-npf — "iugum up: host service supervisor + --container mode".
source "$(dirname "$0")/_lib.sh"; cd "$ROOT"
need curl
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "== build + static checks"
check "static build" env CGO_ENABLED=0 go build -o "$TMP/iugum" .
check "go vet" env CGO_ENABLED=0 go vet ./...
check "unit tests root pkg" env CGO_ENABLED=0 go test ./ ./config/...
check "cmd_up.go exists" test -f cmd_up.go
check "cmd_up_test.go exists" test -f cmd_up_test.go
check_out "usage lists up" '^  up ' "$TMP/iugum" --help
check_out "usage lists container" 'container' "$TMP/iugum" --help
check_out "CONTRIBUTING policy rows: container run" '`container`.*`run`' cat CONTRIBUTING.md
check_out "CONTRIBUTING policy rows: container build" '`container`.*`build`' cat CONTRIBUTING.md
check_out "CONTRIBUTING policy rows: service serve" '`service`.*`serve`' cat CONTRIBUTING.md
check_out "example yaml has container block" '^container:' cat iugum.example.yaml
IUG="$TMP/iugum"
echo "== container mode: dry-run argv"
cd "$TMP"; export IUGUM_DATA="$TMP/data"; mkdir -p "$IUGUM_DATA"
check_out "dry-run docker argv" 'docker run .*--rm' "$IUG" up --container --engine docker --dry-run
check_out "dry-run mounts /workspace" '/workspace' "$IUG" up --container --engine docker --dry-run
check_out "dry-run mounts /data" '/data' "$IUG" up --container --engine docker --dry-run
check_out "dry-run sets IUGUM_DATA" 'IUGUM_DATA=/data' "$IUG" up --container --engine docker --dry-run
check_out "dry-run publishes 3848" '3848' "$IUG" up --container --engine docker --dry-run
check_out "dry-run ends with image then up" 'iugum:latest up$' "$IUG" up --container --engine docker --dry-run
check_out "dry-run --image" 'ghcr.io/x/y:1 up$' "$IUG" up --container --engine docker --image ghcr.io/x/y:1 --dry-run
check_out "dry-run podman" '^podman run' "$IUG" up --container --engine podman --dry-run
check_out "dry-run --name" '--name +demo' "$IUG" up --container --engine docker --name demo --dry-run
check_out "dry-run --detach" ' -d ' "$IUG" up --container --engine docker --detach --dry-run
check_out "container build dry-run" 'build .*--build-arg WITH=claude,codex .*-t iugum:latest' "$IUG" container build --engine docker --with claude,codex --dry-run
check_out "container stop dry-run" 'docker (stop|rm -f) .*iugum' "$IUG" container stop --engine docker --dry-run
check_fail_out "unknown engine rejected" 'engine' "$IUG" up --container --engine rkt --dry-run
echo "== policy gate"
CFG=$(write_deny_policy "$TMP/pol" container run)
check_fail_out "deny container run" 'denied' env IUGUM_CONFIG="$CFG" "$IUG" up --container --engine docker --dry-run
CFG2=$(write_deny_policy "$TMP/pol2" container build)
check_fail_out "deny container build" 'denied' env IUGUM_CONFIG="$CFG2" "$IUG" container build --engine docker --dry-run
CFG3=$(write_deny_policy "$TMP/pol3" service serve)
check_fail_out "deny service serve (host up)" 'denied' env IUGUM_CONFIG="$CFG3" "$IUG" up --wiki-port 0 --observe-port 0 --no-code-server
echo "== host mode: real services"
cd "$TMP"; mkdir -p wiki
WP=$((20000 + RANDOM % 20000)); OP=$((WP+1))
"$IUG" up --wiki-port $WP --observe-port $OP --no-code-server > "$TMP/up.log" 2>&1 &
UPPID=$!
ok=0; for i in $(seq 1 40); do curl -fs "http://127.0.0.1:$OP/meta.json" >/dev/null 2>&1 && ok=1 && break; sleep 0.5; done
if [ $ok = 1 ]; then pass "observe answers on $OP"; else fail "observe did not answer on $OP"; sed 's/^/       | /' "$TMP/up.log" | head -20 >&2; fi
ok=0; for i in $(seq 1 40); do curl -s -o /dev/null "http://127.0.0.1:$WP/" 2>/dev/null && ok=1 && break; sleep 0.5; done
if [ $ok = 1 ]; then pass "wiki answers on $WP"; else fail "wiki did not answer on $WP"; fi
check_out "up.log announces services" 'observe' cat "$TMP/up.log"
kill -TERM $UPPID 2>/dev/null; wait $UPPID; RC=$?
if [ $RC -eq 0 ]; then pass "SIGTERM -> exit 0"; else fail "SIGTERM -> exit $RC"; fi
sleep 1
check_fail "observe port released" curl -fs "http://127.0.0.1:$OP/meta.json"
echo "== host mode: bind failure is non-zero"
"$IUG" up --wiki-port $WP --observe-port $OP --no-code-server > /dev/null 2>&1 & P1=$!
sleep 2
check_fail "second up on same ports exits non-zero" timeout 15 "$IUG" up --wiki-port $WP --observe-port $OP --no-code-server
kill -TERM $P1 2>/dev/null; wait $P1 2>/dev/null
finish
