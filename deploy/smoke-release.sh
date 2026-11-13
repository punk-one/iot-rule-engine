#!/usr/bin/env bash
set -euo pipefail
binary="$(readlink -f "${1:?release executable required}")"
temp="$(mktemp -d /tmp/iot-rule-smoke.XXXXXXXX)"
pid=""
cleanup(){ if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi; case "$temp" in /tmp/iot-rule-smoke.*) rm -rf -- "$temp";; esac; }
trap cleanup EXIT
port="${TEST_ENGINE_PORT:-12091}"
export IOT_RULE_DATA_DIR="$temp/data" IOT_RULE_LISTEN="127.0.0.1:$port"
export IOT_RULE_ADMIN_PASSWORD="Smoke!$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')Aa9"
"$binary" >"$temp/process.log" 2>&1 & pid=$!
ready=0
for attempt in {1..30}; do
  if curl -fsS "http://127.0.0.1:$port/api/v1/health" > /dev/null; then ready=1; break; fi
  kill -0 "$pid" 2>/dev/null || { cat "$temp/process.log"; exit 1; }
  sleep 1
done
[[ "$ready" == 1 ]]
curl -fsS "http://127.0.0.1:$port/" > "$temp/index.html"
grep -q '/static/js/' "$temp/index.html"
script="$(grep -o '/static/js/[^" ]*\.js' "$temp/index.html" | head -1)"
curl -fsS "http://127.0.0.1:$port$script" -o "$temp/script.js"
test -s "$temp/script.js"
[[ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/api/v1/plugins")" == 401 ]]
test -f "$temp/data/rules.db"
echo 'Embedded UI, same-origin API, authentication and fileless SQLite bootstrap passed.'
