#!/usr/bin/env bash
# Validate actual installer parsing before any network or system mutation.
set -euo pipefail
installer="$(cd "$(dirname "$0")" && pwd)/iot-rule-engine.sh"
temp="$(mktemp -d /tmp/iot-rule-installer-test.XXXXXXXX)"
trap 'case "$temp" in /tmp/iot-rule-installer-test.*) rm -rf -- "$temp";; esac' EXIT
mkdir "$temp/tools"
printf '#!/usr/bin/env bash\nexit 88\n' > "$temp/tools/curl"
chmod +x "$temp/tools/curl"
export PATH="$temp/tools:$PATH"
for address in 127.0.0.1:11011 0.0.0.0:12091 '[::1]:11011'; do
  result=0
  bash "$installer" install --no-service --version v1.0.0 --prefix "$temp/install" --data-dir "$temp/data" --listen "$address" >"$temp/log" 2>&1 || result=$?
  [[ "$result" == 88 ]] || { cat "$temp/log"; echo "valid address rejected: $address" >&2;exit 1; }
done
for address in 127.0.0.1:0 127.0.0.1:70000 '::1:11011' host:notaport; do
  result=0
  bash "$installer" install --no-service --version v1.0.0 --prefix "$temp/install" --data-dir "$temp/data" --listen "$address" >"$temp/log" 2>&1 || result=$?
  [[ "$result" == 2 ]] || { echo "invalid address accepted: $address" >&2;exit 1; }
done
echo 'Installer accepts loopback, IPv4 and bracketed IPv6; invalid ports rejected before downloading.'
