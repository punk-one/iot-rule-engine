#!/usr/bin/env bash
# One installation package, independent plugins, signed per-platform releases.
set -euo pipefail
action="${1:-install}"; if (($#)); then shift; fi
version=""; role=edge; listen=127.0.0.1:11011
prefix=/opt/iot-rule-engine; data=/var/lib/iot-rule-engine
factory=""; gateway=""; service=1
# Read only known scalar settings; never execute a saved file as shell code.
args=("$@")
for ((i=0;i<${#args[@]};i++)); do if [[ "${args[i]}" == --prefix ]]; then prefix="${args[i+1]:?}"; fi; done
if [[ -f "$prefix/install.env" ]]; then
  while IFS='=' read -r key value; do
    case "$key" in role) role="$value";;listen) listen="$value";;data) data="$value";;factory) factory="$value";;gateway) gateway="$value";;service) service="$value";; esac
  done < "$prefix/install.env"
fi
while (($#)); do
  case "$1" in
    --version) version="${2:?}"; shift 2;;
    --role) role="${2:?}"; shift 2;;
    --listen) listen="${2:?}"; shift 2;;
    --prefix) prefix="${2:?}"; shift 2;;
    --data-dir) data="${2:?}"; shift 2;;
    --factory-id) factory="${2:?}"; shift 2;;
    --gateway-id) gateway="${2:?}"; shift 2;;
    --no-service) service=0; shift;;
    *) echo "Unknown option: $1" >&2; exit 2;;
  esac
done
case "$role" in edge|server) ;; *) echo 'role must be edge or server' >&2; exit 2;; esac
[[ "$service" == 0 || "$service" == 1 ]] || { echo 'invalid service setting' >&2; exit 2; }
if [[ -z "$gateway" && "$role" == edge ]]; then
  gateway="gateway-$(cat /etc/machine-id 2>/dev/null | sha256sum | cut -c1-12)"
fi
case "$prefix" in /*) ;; *) echo 'prefix must be absolute' >&2; exit 2;; esac
case "$data" in /*) ;; *) echo 'data-dir must be absolute' >&2; exit 2;; esac
for dir in "$prefix" "$data"; do
  [[ "$dir" != / && "$dir" != /home && "$dir" != /root && "$dir" != /usr && "$dir" != /var && "$dir" != *$'\n'* && "$dir" != *' '* ]] || { echo 'unsafe installation directory' >&2; exit 2; }
done
[[ "$listen" =~ ^[a-zA-Z0-9.\[\]:_-]+:[0-9]+$ ]] || { echo 'invalid listen address' >&2; exit 2; }
[[ "$factory" =~ ^[a-zA-Z0-9_-]*$ && "$gateway" =~ ^[a-zA-Z0-9_-]*$ ]] || { echo 'invalid identity' >&2; exit 2; }
if ((service)); then
  [[ $EUID == 0 ]] || { echo 'Use sudo for system service installation.' >&2; exit 1; }
  command -v systemctl >/dev/null || { echo 'systemd required; use --no-service for a portable installation.' >&2; exit 1; }
fi
unit=iot-rule-engine.service
envfile=/etc/iot-rule-engine/bootstrap.env
health_url="http://127.0.0.1:${listen##*:}/api/v1/health"
healthy(){ curl -fsS --max-time 3 "$health_url" >/dev/null; }
wait_health(){ for attempt in {1..30}; do healthy && return 0; sleep 1; done; return 1; }
switch_release(){
  ln -s "$1" "$prefix/.current-$$"
  mv -Tf "$prefix/.current-$$" "$prefix/current"
}
case "$action" in
  status) if ((service)); then systemctl status "$unit"; else "$prefix/current/iot-rule-engine" --version; fi; exit;;
  uninstall)
    if ((service)); then systemctl disable --now "$unit"; rm -f /etc/systemd/system/iot-rule-engine.service; systemctl daemon-reload; fi
    echo "Service removed. Releases and data retained at $prefix and $data."
    exit;;
  rollback)
    previous="$(readlink -f "$prefix/previous")"
    [[ "$previous" == "$prefix/releases/"* && -x "$previous/iot-rule-engine" ]] || { echo 'No previous release.' >&2; exit 1; }
    if ((service)); then systemctl stop "$unit"; fi
    switch_release "$previous"
    if ((service)); then systemctl start "$unit"; wait_health; fi
    echo 'Previous binary restored; use the matching database backup if a schema migration was applied.'
    exit;;
  install|upgrade) ;;
  *) echo 'Usage: iot-rule-engine.sh install|upgrade|rollback|status|uninstall [--version vX.Y.Z] [--role edge|server]' >&2; exit 2;;
esac
[[ "$(uname -s)" == Linux ]] || { echo 'Linux required.' >&2; exit 1; }
case "$(uname -m)" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo 'Supported CPUs: AMD64 and ARM64.' >&2; exit 1;; esac
command -v curl >/dev/null; command -v sha256sum >/dev/null
if [[ -z "$version" ]]; then
  version="$(curl -fsSL --proto '=https' --max-time 30 https://api.github.com/repos/punk-one/iot-rule-engine/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid version.' >&2; exit 1; }
stage="$(mktemp -d /tmp/iot-rule-install.XXXXXXXX)"
trap 'case "$stage" in /tmp/iot-rule-install.*) rm -rf -- "$stage";; esac' EXIT
name="iot-rule-engine-${version}-linux-${arch}"
base="https://github.com/punk-one/iot-rule-engine/releases/download/$version"
for suffix in tar.gz tar.gz.sha256 tar.gz.sigstore.json; do
  curl -fL --proto '=https' --retry 2 --connect-timeout 10 --max-time 300 "$base/$name.$suffix" -o "$stage/$name.$suffix"
done
(cd "$stage" && sha256sum -c "$name.tar.gz.sha256")
verifier="${IOT_RULE_COSIGN_PATH:-}"
if [[ -z "$verifier" ]]; then
  verifier="$(command -v cosign || true)"
fi
if [[ -z "$verifier" ]]; then
  cosign_base=https://github.com/sigstore/cosign/releases/download/v2.4.3
  case "$arch" in
    amd64) verifier_sha=caaad125acef1cb81d58dcdc454a1e429d09a750d1e9e2b3ed1aed8964454708;;
    arm64) verifier_sha=bd0f9763bca54de88699c3656ade2f39c9a1c7a2916ff35601caf23a79be0629;;
  esac
  curl -fsSL --proto '=https' --connect-timeout 10 --max-time 300 "$cosign_base/cosign-linux-$arch" -o "$stage/cosign-linux-$arch"
  printf '%s  %s\n' "$verifier_sha" "$stage/cosign-linux-$arch" | sha256sum -c -
  chmod 0755 "$stage/cosign-linux-$arch"; verifier="$stage/cosign-linux-$arch"
fi
"$verifier" verify-blob "$stage/$name.tar.gz" --bundle "$stage/$name.tar.gz.sigstore.json" \
  --certificate-identity "https://github.com/punk-one/iot-rule-engine/.github/workflows/release.yml@refs/tags/$version" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
# Only the signed archive is ever extracted; publisher identity is fixed above.
tar -xzf "$stage/$name.tar.gz" -C "$stage"
test -x "$stage/$name/iot-rule-engine"
"$stage/$name/iot-rule-engine" --version
mkdir -p "$prefix/releases" "$data/plugins"
mkdir -p "$prefix/tools"
if [[ "$(readlink -f "$verifier")" != "$(readlink -f "$prefix/tools/cosign" || true)" ]]; then install -m 0755 "$verifier" "$prefix/tools/cosign"; fi
previous="$(readlink -f "$prefix/current" || true)"
if [[ ! -d "$prefix/releases/$version" ]]; then mv "$stage/$name" "$prefix/releases/$version"; fi
if ((service)); then
  id iot-rule-engine >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin iot-rule-engine
  mkdir -p /etc/iot-rule-engine
  chmod 0750 /etc/iot-rule-engine
  if [[ ! -f "$data/rules.db" ]]; then
    password="${IOT_RULE_ADMIN_PASSWORD:-}"
    if [[ -z "$password" ]]; then
      password="IoT!$(od -An -N18 -tx1 /dev/urandom | tr -d ' \n')aA9"
      printf 'Initial admin password (shown once): %s\n' "$password" >/dev/tty
    fi
    [[ "$password" =~ ^[a-zA-Z0-9!@#%^_=+:,.?-]{12,128}$ ]] || { echo 'Password must be 12–128 supported characters.' >&2; exit 1; }
    (umask 077; printf 'IOT_RULE_ADMIN_PASSWORD=%s\n' "$password" > "$envfile")
    unset password
  fi
  if [[ -n "$previous" ]]; then
    systemctl stop "$unit"
    backup="$data/backups/$(date -u +%Y%m%dT%H%M%SZ)"
    mkdir -p "$backup"
    for file in rules.db rules.db-wal rules.db-shm; do [[ ! -e "$data/$file" ]] || cp -p "$data/$file" "$backup/"; done
  fi
  chown -R iot-rule-engine:iot-rule-engine "$data"
  chmod 0700 "$data"
  cat > /etc/systemd/system/iot-rule-engine.service <<EOF
[Unit]
Description=IoT Rule Engine
After=network-online.target
Wants=network-online.target
[Service]
User=iot-rule-engine
Group=iot-rule-engine
WorkingDirectory=$data
Environment=IOT_RULE_DATA_DIR=$data
Environment=IOT_RULE_LISTEN=$listen
Environment=IOT_RULE_ROLE=$role
Environment=IOT_RULE_DELIVERY_MODE=reliable
Environment=IOT_RULE_PLUGIN_INSTALL_ENABLED=true
Environment=IOT_RULE_COSIGN_PATH=$prefix/tools/cosign
Environment=IOT_RULE_FACTORY_ID=$factory
Environment=IOT_RULE_GATEWAY_ID=$gateway
Environment=IOT_RULE_INSTANCE_ID=$factory-$gateway-$role
EnvironmentFile=-$envfile
ExecStart=$prefix/current/iot-rule-engine
Restart=on-failure
RestartSec=3
TimeoutStopSec=45
NoNewPrivileges=true
UMask=0077
[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
fi
if [[ -n "$previous" ]]; then ln -sfn "$previous" "$prefix/previous"; fi
switch_release "$prefix/releases/$version"
if ((service)); then
  systemctl enable --now "$unit"
  if ! wait_health; then
    systemctl stop "$unit"
    if [[ -n "$previous" ]]; then
      switch_release "$previous"
      for file in rules.db rules.db-wal rules.db-shm; do
        if [[ -f "$backup/$file" ]]; then cp -p "$backup/$file" "$data/$file"; else rm -f "$data/$file"; fi
      done
      systemctl start "$unit"
    fi
    echo 'New release failed health check; previous release and database restored when available.' >&2; exit 1
  fi
  rm -f "$envfile"
fi
(umask 077; printf 'role=%s\nlisten=%s\ndata=%s\nfactory=%s\ngateway=%s\nservice=%s\n' "$role" "$listen" "$data" "$factory" "$gateway" "$service" > "$prefix/install.env")
printf 'Installed %s (%s). UI/API: %s\n' "$version" "$role" "http://$listen"
if ((!service)); then printf 'Start with IOT_RULE_DATA_DIR=%q IOT_RULE_LISTEN=%q IOT_RULE_DELIVERY_MODE=reliable IOT_RULE_PLUGIN_INSTALL_ENABLED=true IOT_RULE_COSIGN_PATH=%q %q\n' "$data" "$listen" "$prefix/tools/cosign" "$prefix/current/iot-rule-engine"; fi
