#!/bin/bash
#
# package.sh — 打包后端和前端产物为可分发 tar.gz 包
#
# 用法: ./deploy/package.sh [output_dir]
#   默认输出到 deploy/iot-rule-engine.tar.gz
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$SCRIPT_DIR/backend"
FRONTEND_DIR="$SCRIPT_DIR/frontend"
OUTPUT_DIR="${1:-$SCRIPT_DIR}"
PACKAGE_NAME="iot-rule-engine"
PACKAGE_DIR="$SCRIPT_DIR/$PACKAGE_NAME"
TARBALL="$OUTPUT_DIR/$PACKAGE_NAME.tar.gz"

# ─── 检查产物是否存在 ─────────────────────────────────────────────
check_artifacts() {
    local missing=0

    echo -e "${YELLOW}检查打包产物...${NC}"

    if [ ! -f "$BACKEND_DIR/iot-rule-engine" ]; then
        echo -e "${RED}  ✗ 缺少后端二进制: $BACKEND_DIR/iot-rule-engine${NC}"
        missing=1
    else
        echo -e "  ✓ 后端二进制: $(du -h "$BACKEND_DIR/iot-rule-engine" | cut -f1)"
    fi

    if [ ! -d "$BACKEND_DIR/configs" ]; then
        echo -e "${RED}  ✗ 缺少后端配置: $BACKEND_DIR/configs${NC}"
        missing=1
    else
        echo "  ✓ 后端配置"
    fi

    if [ ! -d "$BACKEND_DIR/plugins" ]; then
        echo -e "${RED}  ✗ 缺少后端插件: $BACKEND_DIR/plugins${NC}"
        missing=1
    else
        echo "  ✓ 后端插件 ($(find "$BACKEND_DIR/plugins" -name '*.so' | wc -l) 个)"
    fi

    if [ ! -f "$BACKEND_DIR/install-backend-service.sh" ]; then
        echo -e "${RED}  ✗ 缺少后端安装脚本${NC}"
        missing=1
    else
        echo "  ✓ 后端安装脚本"
    fi

    if [ ! -f "$BACKEND_DIR/iot-rule-engine.service" ]; then
        echo -e "${RED}  ✗ 缺少 service 文件${NC}"
        missing=1
    else
        echo "  ✓ service 文件"
    fi

    if [ ! -d "$FRONTEND_DIR/build" ]; then
        echo -e "${RED}  ✗ 缺少前端构建产物: $FRONTEND_DIR/build${NC}"
        missing=1
    else
        echo "  ✓ 前端构建产物"
    fi

    if [ ! -f "$FRONTEND_DIR/nginx.conf" ]; then
        echo -e "${RED}  ✗ 缺少 nginx.conf${NC}"
        missing=1
    else
        echo "  ✓ nginx.conf"
    fi

    if [ ! -f "$FRONTEND_DIR/docker-compose.yml" ]; then
        echo -e "${RED}  ✗ 缺少 docker-compose.yml${NC}"
        missing=1
    else
        echo "  ✓ docker-compose.yml"
    fi

    if [ "$missing" -eq 1 ]; then
        echo ""
        echo -e "${RED}产物不完整，请先执行 ./deploy/build.sh all${NC}"
        exit 1
    fi

    echo ""
}

# ─── 打包 ──────────────────────────────────────────────────────────
do_package() {
    echo -e "${YELLOW}打包部署包...${NC}"

    # 清理临时目录
    rm -rf "$PACKAGE_DIR"
    mkdir -p "$PACKAGE_DIR"

    # 复制后端
    echo "  复制 backend..."
    mkdir -p "$PACKAGE_DIR/backend"
    rsync -a --exclude 'data/' --exclude 'logs/' "$BACKEND_DIR/" "$PACKAGE_DIR/backend/"

    # 复制前端
    echo "  复制 frontend..."
    mkdir -p "$PACKAGE_DIR/frontend"
    rsync -a "$FRONTEND_DIR/" "$PACKAGE_DIR/frontend/"

    # 复制一键部署脚本
    cp "$SCRIPT_DIR/one-deploy.sh" "$PACKAGE_DIR/"
    chmod +x "$PACKAGE_DIR/one-deploy.sh"

    # 创建 tar.gz
    echo "  创建压缩包..."
    cd "$SCRIPT_DIR"
    tar -czf "$TARBALL" "$PACKAGE_NAME/"

    # 清理临时目录
    rm -rf "$PACKAGE_DIR"

    echo -e "  ${GREEN}✓ 打包完成: $TARBALL${NC}"
    echo "  包大小: $(du -h "$TARBALL" | cut -f1)"
    echo ""
}

# ─── 打印摘要 ──────────────────────────────────────────────────────
print_summary() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}打包完成${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "部署包: $TARBALL"
    echo ""
    echo "使用方法:"
    echo "  1. 拷贝到目标服务器:"
    echo "     scp $TARBALL user@host:/tmp/"
    echo ""
    echo "  2. 在目标服务器解压并安装:"
    echo "     cd /opt"
    echo "     tar -xzf /tmp/iot-rule-engine.tar.gz"
    echo "     cd iot-rule-engine"
    echo "     sudo ./one-deploy.sh"
    echo ""
}

# ─── 主流程 ────────────────────────────────────────────────────────
main() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}IoT Rule Engine 打包${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    check_artifacts
    do_package
    print_summary
}

main "$@"