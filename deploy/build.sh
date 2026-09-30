#!/bin/bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
FRONTEND_DIR="$SCRIPT_DIR/frontend"
BACKEND_DIR="$SCRIPT_DIR/backend"

TARGET="${1:-all}"
API_URL="${REACT_APP_API_URL:-/api/v1}"
FRONTEND_PORT="${FRONTEND_PORT:-3101}"
BACKEND_HOST="${BACKEND_HOST:-host.docker.internal}"
BACKEND_PORT="${BACKEND_PORT:-11011}"
GOOS_TARGET="${GOOS_TARGET:-linux}"
GOARCH_TARGET="${GOARCH_TARGET:-amd64}"
GO_BUILD_CACHE="${GO_BUILD_CACHE:-/tmp/go-build-cache-iot-rule-deploy}"
GO_MOD_CACHE="${GO_MOD_CACHE:-${GOMODCACHE:-$HOME/go/pkg/mod}}"
WORKSPACE_ROOT="$(cd "$PROJECT_ROOT/.." && pwd)"
GO_WORK_FILE="${GO_WORK_FILE:-$WORKSPACE_ROOT/go.work}"
PLUGINS_REPO="${IOT_RULE_PLUGINS_PATH:-$WORKSPACE_ROOT/iot-rule-plugins}"

if [ ! -f "$GO_WORK_FILE" ]; then
    GO_WORK_FILE="off"
fi

usage() {
    cat <<'USAGE'
Usage: ./deploy/build.sh [frontend|backend|plugins|all]
USAGE
}

require_target() {
    case "$TARGET" in
        frontend|backend|plugins|all) ;;
        *)
            usage
            exit 1
            ;;
    esac
}

check_frontend_requirements() {
    command -v node >/dev/null 2>&1 || {
        echo -e "${RED}错误: 未找到 node${NC}"
        exit 1
    }
    command -v npm >/dev/null 2>&1 || {
        echo -e "${RED}错误: 未找到 npm${NC}"
        exit 1
    }
}

check_go_requirements() {
    command -v go >/dev/null 2>&1 || {
        echo -e "${RED}错误: 未找到 go${NC}"
        exit 1
    }
}

check_target_requirements() {
    case "$TARGET" in
        frontend)
            check_frontend_requirements
            ;;
        backend|plugins)
            check_go_requirements
            ;;
        all)
            check_frontend_requirements
            check_go_requirements
            ;;
    esac
}

print_header() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}IoT Rule Engine 本地构建${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo "项目根目录: $PROJECT_ROOT"
    echo "构建目标: $TARGET"
    echo "前端 API 地址: $API_URL"
    echo "前端部署端口: $FRONTEND_PORT"
    echo "前端代理后端: $BACKEND_HOST:$BACKEND_PORT"
    echo ""
}

reset_frontend_output() {
    rm -rf "$FRONTEND_DIR"
    mkdir -p "$FRONTEND_DIR"
}

prepare_backend_layout() {
    mkdir -p "$BACKEND_DIR/configs" "$BACKEND_DIR/plugins" "$BACKEND_DIR/data" "$BACKEND_DIR/logs/rules"
}

reset_backend_output() {
    rm -rf "$BACKEND_DIR"
    prepare_backend_layout
}

build_frontend() {
    echo -e "${YELLOW}编译前端...${NC}"

    cd "$PROJECT_ROOT/ui"

    if [ ! -d node_modules ]; then
        echo "  安装前端依赖..."
        npm install --legacy-peer-deps
    fi

    echo "  执行 React 构建..."
    REACT_APP_API_URL="$API_URL" npm run build

    test -d "$PROJECT_ROOT/ui/build"
    echo "  ✓ 前端编译完成"
    echo ""
}

package_frontend() {
    echo -e "${YELLOW}打包 frontend/...${NC}"

    mkdir -p "$FRONTEND_DIR/build"
    cp -a "$PROJECT_ROOT/ui/build/." "$FRONTEND_DIR/build/"

    cat > "$FRONTEND_DIR/Dockerfile.deploy" <<'DOCKERFILE'
FROM nginx:1.27-alpine

COPY build /usr/share/nginx/html
COPY nginx.conf /etc/nginx/templates/default.conf.template

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
DOCKERFILE

    cat > "$FRONTEND_DIR/nginx.conf" <<'NGINX'
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

upstream backend_server {
    server ${BACKEND_HOST}:${BACKEND_PORT};
}

server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_types text/plain text/css text/xml text/javascript application/javascript application/json application/xml+rss;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://backend_server;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
}
NGINX

    cat > "$FRONTEND_DIR/docker-compose.yml" <<'COMPOSE'
services:
  iot-rule-engine-frontend:
    build:
      context: .
      dockerfile: Dockerfile.deploy
    container_name: iot-rule-engine-frontend
    restart: unless-stopped
    ports:
      - "__FRONTEND_PORT__:80"
    environment:
      BACKEND_HOST: "__BACKEND_HOST__"
      BACKEND_PORT: "__BACKEND_PORT__"
      NGINX_ENVSUBST_FILTER: ^BACKEND_(HOST|PORT)$
    extra_hosts:
      - "host.docker.internal:host-gateway"
COMPOSE

    sed -i "s/__FRONTEND_PORT__/$FRONTEND_PORT/" "$FRONTEND_DIR/docker-compose.yml"
    sed -i "s/__BACKEND_HOST__/$BACKEND_HOST/" "$FRONTEND_DIR/docker-compose.yml"
    sed -i "s/__BACKEND_PORT__/$BACKEND_PORT/" "$FRONTEND_DIR/docker-compose.yml"

    echo "  ✓ frontend 目录已生成"
    echo ""
}

write_backend_service_files() {
    cp -a "$PROJECT_ROOT/configs/." "$BACKEND_DIR/configs/"

    touch "$BACKEND_DIR/data/.keep" "$BACKEND_DIR/logs/.keep" "$BACKEND_DIR/logs/rules/.keep"

    cat > "$BACKEND_DIR/iot-rule-engine.service" <<'SERVICE'
[Unit]
Description=IoT Rule Engine
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/iot-rule-engine/backend
ExecStart=/root/iot-rule-engine/backend/iot-rule-engine -config /root/iot-rule-engine/backend/configs/config.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

    cat > "$BACKEND_DIR/install-backend-service.sh" <<'INSTALL'
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
INSTALL

    chmod +x "$BACKEND_DIR/install-backend-service.sh"
}

build_backend() {
    echo -e "${YELLOW}编译后端...${NC}"

    cd "$PROJECT_ROOT"
    mkdir -p bin

    echo "  优先尝试离线构建..."
    if ! GOWORK="$GO_WORK_FILE" GOPROXY=off GOSUMDB=off GOCACHE="$GO_BUILD_CACHE" GOMODCACHE="$GO_MOD_CACHE" \
        GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" go build -ldflags='-s -w' -o bin/iot-rule-engine ./cmd/main.go; then
        echo "  离线构建失败，回退到在线代理..."
        GOWORK="$GO_WORK_FILE" GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" GOCACHE="$GO_BUILD_CACHE" GOMODCACHE="$GO_MOD_CACHE" \
            GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" go build -ldflags='-s -w' -o bin/iot-rule-engine ./cmd/main.go
    fi

    cp "$PROJECT_ROOT/bin/iot-rule-engine" "$BACKEND_DIR/iot-rule-engine"
    chmod +x "$BACKEND_DIR/iot-rule-engine"
    test -f "$BACKEND_DIR/iot-rule-engine"
    echo "  ✓ 后端编译完成"
    echo ""
}

build_plugins() {
    echo -e "${YELLOW}编译插件...${NC}"

    cd "$PROJECT_ROOT"

    if [ ! -f "$PLUGINS_REPO/build-plugins.sh" ]; then
        echo -e "${RED}错误: 未找到插件构建脚本 $PLUGINS_REPO/build-plugins.sh${NC}"
        exit 1
    fi

    GOWORK="$GO_WORK_FILE" GOCACHE="$GO_BUILD_CACHE" GOMODCACHE="$GO_MOD_CACHE" \
        PLUGINS_DST_DIR="$PROJECT_ROOT/plugins" \
        bash "$PLUGINS_REPO/build-plugins.sh"

    rm -rf "$BACKEND_DIR/plugins"
    mkdir -p "$BACKEND_DIR/plugins"
    cp -a "$PROJECT_ROOT/plugins/." "$BACKEND_DIR/plugins/"

    test -d "$BACKEND_DIR/plugins"
    echo "  ✓ 插件编译完成"
    echo ""
}

run_target() {
    case "$TARGET" in
        frontend)
            reset_frontend_output
            build_frontend
            package_frontend
            ;;
        backend)
            reset_backend_output
            write_backend_service_files
            build_backend
            build_plugins
            ;;
        plugins)
            mkdir -p "$BACKEND_DIR"
            rm -rf "$BACKEND_DIR/plugins"
            mkdir -p "$BACKEND_DIR/plugins"
            build_plugins
            ;;
        all)
            reset_frontend_output
            reset_backend_output
            build_frontend
            package_frontend
            write_backend_service_files
            build_backend
            build_plugins
            ;;
    esac
}

print_summary() {
    echo -e "${GREEN}本地构建完成${NC}"
    echo ""
    if [ -d "$FRONTEND_DIR" ]; then
        echo "前端产物: $FRONTEND_DIR"
    fi
    if [ -d "$BACKEND_DIR" ]; then
        echo "后端产物: $BACKEND_DIR"
    fi
}

main() {
    require_target
    check_target_requirements
    print_header
    run_target
    print_summary
}

main "$@"
