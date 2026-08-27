#!/usr/bin/env bash
# DoD gate for iugum-qps — "iugum net: iptables/nftables network policy through the Casbin gate".
source "$(dirname "$0")/_lib.sh"; cd "$ROOT"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
echo "== build + static checks"
check "static build" env CGO_ENABLED=0 go build -o "$TMP/iugum" .
check "go vet" env CGO_ENABLED=0 go vet ./...
check "adapter/net exists" test -d adapter/net
check "net unit tests" env CGO_ENABLED=0 go test ./adapter/net/... ./ ./config/...
check "cmd_net.go exists" test -f cmd_net.go
check_out "contract has Net interface" 'type Net interface' cat contract/contract.go
check_out "plugin has RegisterNet" 'func RegisterNet' cat plugin/plugin.go
check_out "app has ApplyNet" 'func \(a \*App\) ApplyNet' cat app/*.go
check_out "usage lists net" '^  net ' "$TMP/iugum" --help
check_out "CONTRIBUTING rows: net plan" '`net`.*`plan`' cat CONTRIBUTING.md
check_out "CONTRIBUTING rows: net apply" '`net`.*`apply`' cat CONTRIBUTING.md
check_out "CONTRIBUTING rows: net show" '`net`.*`show`' cat CONTRIBUTING.md
check_out "example yaml has network block" '^network:' cat iugum.example.yaml
IUG="$TMP/iugum"
cat > "$TMP/rules.yaml" <<'Y'
network:
  backend: iptables
  default:
    in: deny
    out: allow
  rules:
    - name: wiki-in
      dir: in
      proto: tcp
      port: 3000
      src: 203.0.113.0/24
      action: allow
    - name: observe-in
      dir: in
      proto: tcp
      port: 3848
      action: allow
    - name: no-smtp-out
      dir: out
      proto: tcp
      port: 25
      action: deny
Y
sed 's/backend: iptables/backend: nftables/' "$TMP/rules.yaml" > "$TMP/rules-nft.yaml"
echo "== plan rendering"
check_out "iptables plan renders -A" 'iptables -A' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net plan
check_out "iptables plan has port 3000" '3000' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net plan
check_out "iptables plan has src cidr" '203\.0\.113\.0/24' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net plan
check_out "iptables plan denies smtp out" '25' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net plan
check_out "iptables plan sets INPUT default deny" 'INPUT DROP|-P INPUT DROP|INPUT.*DROP' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net plan
check_out "nftables plan renders table" 'table inet' env IUGUM_CONFIG="$TMP/rules-nft.yaml" "$IUG" net plan
check_out "nftables plan has dport 3848" 'dport 3848' env IUGUM_CONFIG="$TMP/rules-nft.yaml" "$IUG" net plan
check_out "apply --dry-run prints plan" 'iptables -A' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net apply --dry-run
echo "== absent / off"
printf 'network:\n  backend: off\n' > "$TMP/off.yaml"
check "plan with backend off exits 0" env IUGUM_CONFIG="$TMP/off.yaml" "$IUG" net plan
check "plan with no network block exits 0" env IUGUM_CONFIG=/dev/null "$IUG" net plan
echo "== policy gate"
CFG=$(write_deny_policy "$TMP/pol" net apply); cat "$TMP/rules.yaml" >> "$CFG"
check_fail_out "deny net apply" 'denied' env IUGUM_CONFIG="$CFG" "$IUG" net apply --dry-run
check_out "plan still allowed" 'iptables -A' env IUGUM_CONFIG="$CFG" "$IUG" net plan
CFG2=$(write_deny_policy "$TMP/pol2" net plan); cat "$TMP/rules.yaml" >> "$CFG2"
check_fail_out "deny net plan" 'denied' env IUGUM_CONFIG="$CFG2" "$IUG" net plan
echo "== darwin: apply is linux only"
if [ "$(uname)" = Darwin ]; then
  check_fail_out "apply on darwin errors with linux" '[Ll]inux' env IUGUM_CONFIG="$TMP/rules.yaml" "$IUG" net apply
fi
echo "== linux: real apply in a container (needs docker)"
need docker; docker info >/dev/null 2>&1 || { echo "docker daemon not reachable" >&2; exit 2; }
check "linux static build" env CGO_ENABLED=0 GOOS=linux go build -o "$TMP/iugum-linux" .
cp "$TMP/rules.yaml" "$TMP/rules-nft.yaml" "$TMP/lin/" 2>/dev/null || { mkdir -p "$TMP/lin"; cp "$TMP/rules.yaml" "$TMP/rules-nft.yaml" "$TMP/lin/"; }
cp "$TMP/iugum-linux" "$TMP/lin/iugum"
run_lin() { docker run --rm --cap-add NET_ADMIN -v "$TMP/lin:/w" -w /w -e IUGUM_CONFIG="$1" alpine:3.20 sh -c "apk add -q iptables nftables >/dev/null 2>&1; $2"; }
check_out "iptables apply inside linux" 'dpt:3000|3000' run_lin /w/rules.yaml './iugum net apply && iptables -S && iptables -L -n'
check_out "iptables show inside linux" '3848' run_lin /w/rules.yaml './iugum net apply >/dev/null && ./iugum net show'
check_out "nftables apply inside linux" 'dport 3848' run_lin /w/rules-nft.yaml './iugum net apply && nft list ruleset'
check_fail_out "apply without NET_ADMIN refuses" 'NET_ADMIN|permission|not permitted' docker run --rm --cap-drop ALL -v "$TMP/lin:/w" -w /w -e IUGUM_CONFIG=/w/rules.yaml alpine:3.20 sh -c 'apk add -q iptables >/dev/null 2>&1; ./iugum net apply'
finish
