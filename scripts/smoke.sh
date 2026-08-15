#!/bin/sh
# ⚡ The pressure-plate test — step on every circuit of a RUNNING redstone.
# Run after every redstone edit or deploy:
#   REDSTONE_URL=http://localhost:8215 ./smoke.sh
# Unit tests (go test) guard the logic; this guards the deployment: transport
# Expects the reference registry fixtures (minio-local, the ipfs-overclaimed
# liar, the prod/admin wired edge) — the my-stack deployment, or your own
# registry with equivalent stacks.
# up, registry loaded, verdicts enforced, edges served.
set -u
REDSTONE_URL="${REDSTONE_URL:-http://localhost:8215}"
pass=0; fail=0

check() { # name, expected, actual
  if [ "$2" = "$3" ]; then pass=$((pass+1)); echo "ok   $1"
  else fail=$((fail+1)); echo "FAIL $1: expected [$2] got [$3]"; fi
}
get()  { curl -s --max-time 15 "$REDSTONE_URL$1"; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 15 "$REDSTONE_URL$1"; }
jgrep(){ get "$1" | grep -o "$2" | head -1; }

# NOTE: expectations assume the kernel runs with --strict (our deployment).
check "health"                 '"ok": true' "$(jgrep /health '"ok": true')"
check "happy bind verified"    '"name": "minio-local"' "$(jgrep '/bind?capability=blob&level=mutable&consumer=smoke&as=t1' '"name": "[^"]*"')"
check "declaration-driven"     '"name": "minio-local"' "$(jgrep '/bind?stack=prod&consumer=reporter&as=uploads' '"name": "[^"]*"')"
check "liar refused"           "409" "$(code '/bind?capability=blob&level=mutable&name=ipfs-overclaimed')"
check "liar reason"            '"reason": "claims mutable, conformance verified only core"' "$(jgrep '/bind?capability=blob&level=mutable&name=ipfs-overclaimed' '"reason": "[^"]*"')"
check "unknown level 400"      "400" "$(code '/bind?capability=blob&level=nonsense')"
check "undeclared no-cap 400"  "400" "$(code '/bind?stack=prod&consumer=nobody&as=mystery')"
check "traversal refused"      "400" "$(code '/bind?capability=blob&level=mutable&stack=..%2F..%2Fetc&consumer=x&as=x&name=nothere')"
check "strict undeclared 400"  "400" "$(code '/bind?capability=blob&level=mutable&stack=prod&consumer=reporter&as=freestyle')"
check "graph served"           "edges" "$(get /graph | grep -o '"edges"' | head -1 | tr -d '"')"
check "core self-manifest"     '"capability": "registry"' "$(jgrep '/svc/registry/manifest' '"capability": "[^"]*"')"
check "verdicts served"        '"results"' "$(get /svc/conformance/verdicts | grep -o '"results"' | head -1)"
check "edge lookup (with)"     '"space": "admin"' "$(jgrep '/svc/registry/edge?stack=prod&app=admin&task=blob' '"space": "[^"]*"')"
check "instances >= 15"        "yes" "$([ "$(get /instances | grep -c '"name"')" -ge 15 ] && echo yes || echo no)"

echo "---"
if [ "$fail" -eq 0 ]; then
  echo "⚡ all circuits power ($pass checks)"
else
  echo "🧨 $fail dead circuit(s), $pass alive"
fi
[ "$fail" -eq 0 ]
