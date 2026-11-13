#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off
version="${VERSION:?VERSION is required}"
arch="${GOARCH:-$(go env GOHOSTARCH)}"
case "$arch" in amd64|arm64) ;; *) echo 'unsupported architecture' >&2; exit 1;; esac
if [[ "${SKIP_UI_BUILD:-0}" != 1 ]]; then (cd ui && npm ci && npm run build); fi
test -f ui/build/index.html
mkdir -p internal/webui/assets
cp -R ui/build/. internal/webui/assets/
package="iot-rule-engine-${version}-linux-${arch}"
root="dist/$package"
mkdir -p "$root"
commit="$(git rev-parse HEAD)"
built="$(date -u +%FT%TZ)"
GOOS=linux GOARCH="$arch" CGO_ENABLED=1 go build -trimpath \
  -ldflags "-s -w ${EXTRA_LDFLAGS:-} -X github.com/punk-one/iot-rule-engine/internal/version.Version=$version -X github.com/punk-one/iot-rule-engine/internal/version.Commit=$commit -X github.com/punk-one/iot-rule-engine/internal/version.BuiltAt=$built" \
  -o "$root/iot-rule-engine" ./cmd
cp LICENSE "$root/"
cp deploy/iot-rule-engine.sh "$root/"
go version -m "$root/iot-rule-engine" > "$root/BUILDINFO"
tar -C dist -czf "dist/$package.tar.gz" "$package"
(cd dist && sha256sum "$package.tar.gz" > "$package.tar.gz.sha256")
if [[ "${RELEASE_SIGN:-0}" == 1 ]]; then
  cosign sign-blob --yes --bundle "dist/$package.tar.gz.sigstore.json" "dist/$package.tar.gz"
fi
printf '%s\n' "dist/$package.tar.gz"
