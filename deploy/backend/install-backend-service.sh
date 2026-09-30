#!/bin/bash

set -euo pipefail

BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_SRC="$BACKEND_DIR/iot-rule-engine.service"
SERVICE_DST="/etc/systemd/system/iot-rule-engine.service"

if [ ! -f "$BACKEND_DIR/iot-rule-engine" ]; then
    echo "backend 二进制不存在: $BACKEND_DIR/iot-rule-engine" >&2
    exit 1
fi

mkdir -p "$BACKEND_DIR/data" "$BACKEND_DIR/logs/rules"
chmod +x "$BACKEND_DIR/iot-rule-engine"

cp "$SERVICE_SRC" "$SERVICE_DST"
chmod 644 "$SERVICE_DST"

systemctl daemon-reload
systemctl enable iot-rule-engine
systemctl restart iot-rule-engine
systemctl status iot-rule-engine --no-pager --full || true
