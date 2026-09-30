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

# ─── 加载同目录 .env 文件 ────────────────────────────────────────
ENV_FILE="$SCRIPT_DIR/$(basename "${BASH_SOURCE[0]}" .sh).env"
if [ -f "$ENV_FILE" ]; then
    echo -e "${YELLOW}加载环境配置: $ENV_FILE${NC}"
    set -a
    # shellcheck source=/dev/null
    . "$ENV_FILE"
    set +a
else
    echo -e "${YELLOW}未找到 $ENV_FILE，使用默认值或环境变量${NC}"
fi

# ─── 配置参数（环境变量 → 默认值）────────────────────────────────
TARGET="${1:-all}"
SKIP_BUILD="${SKIP_BUILD:-0}"
DEPLOY_MODE="${DEPLOY_MODE:-deploy}"
REMOTE_HOST="${REMOTE_HOST:?REMOTE_HOST is required}"
REMOTE_PORT="${REMOTE_PORT:-22}"
REMOTE_USER="${REMOTE_USER:-deploy}"
REMOTE_ROOT="${REMOTE_ROOT:-/opt/iot-rule-engine}"
FRONTEND_PORT="${FRONTEND_PORT:-3101}"
BACKEND_PORT="${BACKEND_PORT:-11011}"
SSH_TARGET="${REMOTE_USER}@${REMOTE_HOST}"
SSH_OPTS=(-p "$REMOTE_PORT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new)
RSYNC_SSH="ssh -p $REMOTE_PORT -o BatchMode=yes -o StrictHostKeyChecking=accept-new"

# ─── 用法 ─────────────────────────────────────────────────────────
usage() {
    cat <<USAGE
用法: ./deploy/deploy-to-server.sh [frontend|backend|plugins|all] [deploy|update]

  第一个参数: 部署目标 (frontend/backend/plugins/all)
  第二个参数: 部署模式 (deploy/update，默认从 env 文件读取，否则为 deploy)

模式说明:
  deploy  全量部署 — 上传所有产物、安装服务、启动
  update  增量升级 — 仅上传后端二进制 + 前端 dist，重启服务

环境配置:
  与脚本同目录下的 .env 文件会被自动加载。
  示例: cp deploy-to-server.env.tpl deploy-to-server.env 然后编辑

USAGE
}

# ─── 参数解析 ─────────────────────────────────────────────────────
parse_args() {
    case "$TARGET" in
        frontend|backend|plugins|all) ;;
        deploy|update)
            # 第一个参数实际上是部署模式，目标默认为 all
            DEPLOY_MODE="$TARGET"
            TARGET="${2:-all}"
            ;;
        *)
            usage
            exit 1
            ;;
    esac

    # 第二个可选参数覆盖部署模式
    if [ "${2:-}" = "deploy" ] || [ "${2:-}" = "update" ]; then
        DEPLOY_MODE="$2"
    fi
}

# ─── 检查环境依赖 ─────────────────────────────────────────────────
check_requirements() {
    command -v ssh >/dev/null 2>&1 || { echo -e "${RED}错误: 未找到 ssh${NC}"; exit 1; }
    command -v rsync >/dev/null 2>&1 || { echo -e "${RED}错误: 未找到 rsync${NC}"; exit 1; }
}

# ─── 打印头部 ─────────────────────────────────────────────────────
print_header() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}IoT Rule Engine 远端部署${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo "项目根目录: $PROJECT_ROOT"
    echo "部署目标: $TARGET"
    echo "部署模式: $DEPLOY_MODE"
    echo "目标服务器: $SSH_TARGET:$REMOTE_ROOT"
    echo "SKIP_BUILD: $SKIP_BUILD"
    echo ""
}

# ─── 检查本地产物 ─────────────────────────────────────────────────
ensure_local_artifacts() {
    case "$TARGET" in
        frontend)
            test -f "$FRONTEND_DIR/docker-compose.yml" || { echo "缺少 frontend 产物: $FRONTEND_DIR/docker-compose.yml" >&2; exit 1; }
            test -f "$FRONTEND_DIR/Dockerfile.deploy" || { echo "缺少 frontend 产物: $FRONTEND_DIR/Dockerfile.deploy" >&2; exit 1; }
            test -d "$FRONTEND_DIR/build" || { echo "缺少 frontend 产物: $FRONTEND_DIR/build" >&2; exit 1; }
            ;;
        backend)
            test -f "$BACKEND_DIR/iot-rule-engine" || { echo "缺少 backend 二进制: $BACKEND_DIR/iot-rule-engine" >&2; exit 1; }
            test -d "$BACKEND_DIR/configs" || { echo "缺少 backend 配置: $BACKEND_DIR/configs" >&2; exit 1; }
            test -d "$BACKEND_DIR/plugins" || { echo "缺少 backend/plugins 插件: $BACKEND_DIR/plugins" >&2; exit 1; }
            test -f "$BACKEND_DIR/install-backend-service.sh" || { echo "缺少 backend 安装脚本: $BACKEND_DIR/install-backend-service.sh" >&2; exit 1; }
            ;;
        plugins)
            test -d "$BACKEND_DIR/plugins" || { echo "缺少 backend/plugins 产物: $BACKEND_DIR/plugins" >&2; exit 1; }
            ;;
        all)
            test -f "$FRONTEND_DIR/docker-compose.yml" || { echo "缺少 frontend 产物: $FRONTEND_DIR/docker-compose.yml" >&2; exit 1; }
            test -d "$FRONTEND_DIR/build" || { echo "缺少 frontend 产物: $FRONTEND_DIR/build" >&2; exit 1; }
            test -f "$BACKEND_DIR/iot-rule-engine" || { echo "缺少 backend 二进制: $BACKEND_DIR/iot-rule-engine" >&2; exit 1; }
            test -d "$BACKEND_DIR/plugins" || { echo "缺少 backend/plugins 产物: $BACKEND_DIR/plugins" >&2; exit 1; }
            ;;
    esac
}

# ─── 本地构建 ─────────────────────────────────────────────────────
run_local_build_if_needed() {
    if [ "$SKIP_BUILD" = "1" ]; then
        echo -e "${YELLOW}跳过本地构建，直接复用现有产物${NC}"
        ensure_local_artifacts
    else
        "$SCRIPT_DIR/build.sh" "$TARGET"
        ensure_local_artifacts
    fi
}

# ─── SSH 密钥认证辅助函数 ─────────────────────────────────────────
run_remote_ssh() {
    ssh "${SSH_OPTS[@]}" "$SSH_TARGET" "$1"
}

run_remote_rsync() {
    rsync "$@"
}

# ═══════════════════════════════════════════════════════════════════
# deploy 模式 — 全量部署
# ═══════════════════════════════════════════════════════════════════

prepare_remote_dirs() {
    echo -e "${YELLOW}创建远端目录结构...${NC}"
    run_remote_ssh "mkdir -p '$REMOTE_ROOT/frontend' '$REMOTE_ROOT/backend' '$REMOTE_ROOT/backend/configs' '$REMOTE_ROOT/backend/plugins' '$REMOTE_ROOT/backend/data' '$REMOTE_ROOT/backend/logs/rules'"
    echo "  ✓ 目录已创建"
    echo ""
}

stop_remote_target() {
    echo -e "${YELLOW}停止远端目标服务...${NC}"
    case "$TARGET" in
        frontend)
            run_remote_ssh "cd '$REMOTE_ROOT/frontend' 2>/dev/null && docker compose down || true"
            ;;
        backend|plugins)
            run_remote_ssh "systemctl stop iot-rule-engine || true"
            ;;
        all)
            run_remote_ssh "systemctl stop iot-rule-engine || true; cd '$REMOTE_ROOT/frontend' 2>/dev/null && docker compose down || true"
            ;;
    esac
    echo "  ✓ 远端停止命令已执行"
    echo ""
}

upload_frontend() {
    run_remote_rsync -az --delete -e "$RSYNC_SSH" \
        "$FRONTEND_DIR/" "$SSH_TARGET:$REMOTE_ROOT/frontend/"
}

upload_backend_core() {
    run_remote_rsync -az -e "$RSYNC_SSH" \
        "$BACKEND_DIR/iot-rule-engine" "$SSH_TARGET:$REMOTE_ROOT/backend/iot-rule-engine"
    run_remote_rsync -az --delete -e "$RSYNC_SSH" \
        "$BACKEND_DIR/configs/" "$SSH_TARGET:$REMOTE_ROOT/backend/configs/"
    run_remote_rsync -az --delete -e "$RSYNC_SSH" \
        "$BACKEND_DIR/plugins/" "$SSH_TARGET:$REMOTE_ROOT/backend/plugins/"
    run_remote_rsync -az -e "$RSYNC_SSH" \
        "$BACKEND_DIR/iot-rule-engine.service" "$SSH_TARGET:$REMOTE_ROOT/backend/iot-rule-engine.service"
    run_remote_rsync -az -e "$RSYNC_SSH" \
        "$BACKEND_DIR/install-backend-service.sh" "$SSH_TARGET:$REMOTE_ROOT/backend/install-backend-service.sh"
}

upload_backend_plugins() {
    run_remote_rsync -az --delete -e "$RSYNC_SSH" \
        "$BACKEND_DIR/plugins/" "$SSH_TARGET:$REMOTE_ROOT/backend/plugins/"
}

upload_all_backend() {
    run_remote_rsync -az --delete --exclude 'data/' --exclude 'logs/' -e "$RSYNC_SSH" \
        "$BACKEND_DIR/" "$SSH_TARGET:$REMOTE_ROOT/backend/"
}

upload_target() {
    echo -e "${YELLOW}上传目标产物到远端...${NC}"
    prepare_remote_dirs
    case "$TARGET" in
        frontend)
            upload_frontend
            ;;
        backend)
            upload_backend_core
            ;;
        plugins)
            upload_backend_plugins
            ;;
        all)
            upload_frontend
            upload_all_backend
            ;;
    esac
    echo "  ✓ 上传完成"
    echo ""
}

start_remote_target() {
    echo -e "${YELLOW}启动远端目标服务...${NC}"
    case "$TARGET" in
        frontend)
            run_remote_ssh "cd '$REMOTE_ROOT/frontend' && docker compose up -d --build && docker compose ps"
            ;;
        backend)
            run_remote_ssh "chmod +x '$REMOTE_ROOT/backend/install-backend-service.sh' && '$REMOTE_ROOT/backend/install-backend-service.sh'"
            ;;
        plugins)
            run_remote_ssh "systemctl start iot-rule-engine && systemctl status iot-rule-engine --no-pager --full"
            ;;
        all)
            run_remote_ssh "chmod +x '$REMOTE_ROOT/backend/install-backend-service.sh' && '$REMOTE_ROOT/backend/install-backend-service.sh'"
            run_remote_ssh "cd '$REMOTE_ROOT/frontend' && docker compose up -d --build && docker compose ps"
            ;;
    esac
    echo "  ✓ 远端启动命令已执行"
    echo ""
}

# ═══════════════════════════════════════════════════════════════════
# update 模式 — 增量升级
# ═══════════════════════════════════════════════════════════════════

update_remote() {
    echo -e "${YELLOW}[update] 增量升级远端服务...${NC}"

    # 确保远端目标目录存在
    run_remote_ssh "mkdir -p '$REMOTE_ROOT/backend' '$REMOTE_ROOT/frontend'"

    case "$TARGET" in
        frontend)
            # 仅更新前端 dist
            echo "  上传前端 dist..."
            run_remote_rsync -az --delete -e "$RSYNC_SSH" \
                "$FRONTEND_DIR/build/" "$SSH_TARGET:$REMOTE_ROOT/frontend/build/"
            echo "  重启前端..."
            # 尝试 docker compose 重启，如果未使用 docker 则仅重启 nginx
            if run_remote_ssh "cd '$REMOTE_ROOT/frontend' 2>/dev/null && test -f docker-compose.yml 2>/dev/null"; then
                run_remote_ssh "cd '$REMOTE_ROOT/frontend' && docker compose up -d --build"
            else
                run_remote_ssh "systemctl reload nginx || systemctl restart nginx || true"
            fi
            ;;
        backend)
            # 更新后端二进制 + 插件
            echo "  上传后端二进制..."
            run_remote_rsync -az -e "$RSYNC_SSH" \
                "$BACKEND_DIR/iot-rule-engine" "$SSH_TARGET:$REMOTE_ROOT/backend/iot-rule-engine"
            echo "  上传插件..."
            run_remote_rsync -az --delete -e "$RSYNC_SSH" \
                "$BACKEND_DIR/plugins/" "$SSH_TARGET:$REMOTE_ROOT/backend/plugins/"
            echo "  重启后端服务..."
            run_remote_ssh "chmod +x '$REMOTE_ROOT/backend/iot-rule-engine' && systemctl restart iot-rule-engine"
            ;;
        plugins)
            # 增量更新插件相当于重传所有插件
            echo "  上传插件..."
            run_remote_rsync -az --delete -e "$RSYNC_SSH" \
                "$BACKEND_DIR/plugins/" "$SSH_TARGET:$REMOTE_ROOT/backend/plugins/"
            echo "  重启后端服务..."
            run_remote_ssh "systemctl restart iot-rule-engine"
            ;;
        all)
            # 更新后端二进制 + 前端 dist
            echo "  上传后端二进制..."
            run_remote_rsync -az -e "$RSYNC_SSH" \
                "$BACKEND_DIR/iot-rule-engine" "$SSH_TARGET:$REMOTE_ROOT/backend/iot-rule-engine"
            echo "  上传前端 dist..."
            run_remote_rsync -az --delete -e "$RSYNC_SSH" \
                "$FRONTEND_DIR/build/" "$SSH_TARGET:$REMOTE_ROOT/frontend/build/"
            echo "  重启后端服务..."
            run_remote_ssh "chmod +x '$REMOTE_ROOT/backend/iot-rule-engine' && systemctl restart iot-rule-engine"
            echo "  重启前端..."
            if run_remote_ssh "cd '$REMOTE_ROOT/frontend' 2>/dev/null && test -f docker-compose.yml 2>/dev/null"; then
                run_remote_ssh "cd '$REMOTE_ROOT/frontend' && docker compose up -d --build"
            else
                run_remote_ssh "systemctl reload nginx || systemctl restart nginx || true"
            fi
            ;;
    esac
    echo "  ✓ 增量升级完成"
    echo ""
}

# ═══════════════════════════════════════════════════════════════════
# 打印摘要
# ═══════════════════════════════════════════════════════════════════

print_summary() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}部署完成${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "部署模式: $([ "$DEPLOY_MODE" = "update" ] && echo "增量升级" || echo "全量部署")"
    echo "部署目标: $TARGET"
    echo "目标服务器: $SSH_TARGET:$REMOTE_ROOT"
    echo ""
    echo "本地产物目录:"
    echo "  - $FRONTEND_DIR"
    echo "  - $BACKEND_DIR"
    echo ""
    echo "远端目录:"
    echo "  - $SSH_TARGET:$REMOTE_ROOT/frontend"
    echo "  - $SSH_TARGET:$REMOTE_ROOT/backend"
    echo ""
}

# ═══════════════════════════════════════════════════════════════════
# 主流程
# ═══════════════════════════════════════════════════════════════════

main() {
    parse_args
    check_requirements
    print_header
    run_local_build_if_needed

    if [ "$DEPLOY_MODE" = "update" ]; then
        update_remote
    else
        stop_remote_target
        upload_target
        start_remote_target
    fi

    print_summary
}

main "$@"
