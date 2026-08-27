#!/usr/bin/env bash
# DoD gate for iugum-uxo — "Dockerfile: fat image with WITH build args".
source "$(dirname "$0")/_lib.sh"; cd "$ROOT"
need docker
docker info >/dev/null 2>&1 || { echo "docker daemon not reachable" >&2; exit 2; }
echo "== files"
check "Dockerfile exists" test -f Dockerfile
check ".dockerignore exists" test -f .dockerignore
check_out ".dockerignore excludes .git" '^\.git' cat .dockerignore
check_out ".dockerignore excludes node_modules" 'node_modules' cat .dockerignore
check "docs/container.md exists" test -f docs/container.md
check_out "docs mention WITH" 'WITH' cat docs/container.md
check_out "docs mention CGO_ENABLED" 'CGO_ENABLED' cat docs/container.md
check_out "README links container doc" 'docs/container.md' cat README.md
check_out "Dockerfile has ARG WITH" '^ARG WITH' cat Dockerfile
check_out "Dockerfile has ARG CGO_ENABLED" '^ARG CGO_ENABLED' cat Dockerfile
check_out "Dockerfile installs libicu-dev in builder" 'libicu-dev' cat Dockerfile
check_out "Dockerfile ENTRYPOINT iugum" 'ENTRYPOINT \["iugum"\]' cat Dockerfile
check_out "Dockerfile CMD up" 'CMD \["up"\]' cat Dockerfile
check_out "Dockerfile uid 1000 user" '1000' cat Dockerfile
check "host static build still compiles" env CGO_ENABLED=0 go build -o /tmp/iugum-dod-uxo .

echo "== image 1: WITH=none CGO_ENABLED=0"
T1=iugum-dod:none
check "build WITH=none" docker build --build-arg WITH=none --build-arg CGO_ENABLED=0 -t $T1 .
check_out "iugum --help runs" 'Usage: iugum' docker run --rm $T1 --help
check_fail "claude absent" docker run --rm --entrypoint sh $T1 -c 'command -v claude'
check_fail "code-server absent" docker run --rm --entrypoint sh $T1 -c 'command -v code-server'
check_out "runs as uid 1000" '^1000$' docker run --rm --entrypoint id $T1 -u
check_out "IUGUM_DATA=/data" '/data' docker run --rm --entrypoint sh $T1 -c 'echo $IUGUM_DATA'
check_out "/workspace writable" 'ok' docker run --rm --entrypoint sh $T1 -c 'touch /workspace/.w && echo ok'
check_out "/data writable" 'ok' docker run --rm --entrypoint sh $T1 -c 'touch /data/.w && echo ok'
check_out "label iugum.with=none" 'none' docker inspect -f '{{index .Config.Labels "iugum.with"}}' $T1
check_out "label image.source" 'iugum' docker inspect -f '{{index .Config.Labels "org.opencontainers.image.source"}}' $T1

echo "== image 2: WITH=claude,code-server CGO_ENABLED=1"
T2=iugum-dod:claude-cs
check "build WITH=claude,code-server CGO=1" docker build --build-arg WITH=claude,code-server --build-arg CGO_ENABLED=1 -t $T2 .
check_out "iugum --help runs (cgo)" 'Usage: iugum' docker run --rm $T2 --help
check "claude --version" docker run --rm --entrypoint claude $T2 --version
check "code-server --version" docker run --rm --entrypoint code-server $T2 --version
check_fail "codex absent" docker run --rm --entrypoint sh $T2 -c 'command -v codex'
check_fail "opencode absent" docker run --rm --entrypoint sh $T2 -c 'command -v opencode'
check_fail "cursor-agent absent" docker run --rm --entrypoint sh $T2 -c 'command -v cursor-agent'
check_out "label iugum.with lists both" 'claude' docker inspect -f '{{index .Config.Labels "iugum.with"}}' $T2
# embedded dolt must not be the "requires a CGO build" error in the cgo image
OUT=$(docker run --rm -w /tmp $T2 beads list 2>&1 || true)
if printf '%s' "$OUT" | grep -q 'requires a CGO build'; then fail "cgo image still says 'requires a CGO build'"; else pass "cgo image has embedded dolt linked"; fi
finish
