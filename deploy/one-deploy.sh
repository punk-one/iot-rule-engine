#!/bin/bash
#
# one-deploy.sh — 一键安装 IoT Rule Engine（后端 + 前端）
#
# 用法: sudo ./one-deploy.sh [options]
#
# 选项:
#   --frontend-port PORT   前端监听端口 (默认: 3101)
#   --backend-host HOST    后端 API 地址 (默认: 127.0.0.1)
#   --backend-port PORT    后端 API 端口 (默认: 11011)
#   --help                 显示帮助信息
#
# 说明:
#   必须与 backend/ 和 frontend/ 目录同级运行。
#   后端安装为 systemd 服务，前端通过 Docker Compose 部署。
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ─── 默认配置 ─────────────────────────────────────────────────────
FRONTEND_PORT=3101
BACKEND_HOST=127.0.0.1
BACKEND_PORT=11011
INSTALL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ─── 参数解析 ─────────────────────────────────────────────────────
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --frontend-port)
                FRONTEND_PORT="$2"; shift 2 ;;
            --backend-host)
                BACKEND_HOST="$2"; shift 2 ;;
            --backend-port)
                BACKEND_PORT="$2"; shift 2 ;;
            --help|-h)
                sed -n '/^#/,/^$/p' "$0" | sed 's/^# \?//'; exit 0 ;;
            *)
                echo -e "${RED}错误: 未知参数 $1${NC}" >&2
                echo "用法: sudo ./one-deploy.sh [--frontend-port PORT] [--backend-host HOST] [--backend-port PORT]" >&2
                exit 1 ;;
        esac
    done
}

# ─── 检查环境 ─────────────────────────────────────────────────────
check_requirements() {
    echo -e "${YELLOW}检查环境...${NC}"

    if [ "$(id -u)" -ne 0 ]; then
        echo -e "${RED}错误: 请以 root 用户或使用 sudo 运行${NC}"
        exit 1
    fi

    if [ ! -d "$INSTALL_DIR/backend" ]; then
        echo -e "${RED}错误: 未找到 backend 目录 ($INSTALL_DIR/backend)${NC}"
        echo "请确保 one-deploy.sh 与 backend/ 目录同级" >&2
        exit 1
    fi

    if [ ! -d "$INSTALL_DIR/frontend" ]; then
        echo -e "${RED}错误: 未找到 frontend 目录 ($INSTALL_DIR/frontend)${NC}"
        echo "请确保 one-deploy.sh 与 frontend/ 目录同级" >&2
        exit 1
    fi

    if [ ! -f "$INSTALL_DIR/backend/iot-rule-engine" ]; then
        echo -e "${RED}错误: 后端二进制不存在: $INSTALL_DIR/backend/iot-rule-engine${NC}"
        exit 1
    fi

    echo "  ✓ 目录结构完整"
    echo "  安装目录: $INSTALL_DIR"
    echo "  前端端口: $FRONTEND_PORT"
    echo "  后端地址: $BACKEND_HOST:$BACKEND_PORT"
    echo ""

    check_docker_mirror
}

# ─── 安装后端 ─────────────────────────────────────────────────────
install_backend() {
    echo -e "${YELLOW}安装后端服务...${NC}"

    local backend_dir="$INSTALL_DIR/backend"

    # 创建数据目录
    mkdir -p "$backend_dir/data" "$backend_dir/logs/rules"

    # 设置二进制可执行权限
    chmod +x "$backend_dir/iot-rule-engine"

    # 配置 service 文件中的路径
    local service_file="$backend_dir/iot-rule-engine.service"
    if [ -f "$service_file" ]; then
        sed -i "s|^WorkingDirectory=.*|WorkingDirectory=$backend_dir|" "$service_file"
        sed -i "s|^ExecStart=.*|ExecStart=$backend_dir/iot-rule-engine -config $backend_dir/configs/config.yaml|" "$service_file"
    fi

    # 安装 service
    cp "$service_file" /etc/systemd/system/iot-rule-engine.service
    chmod 644 /etc/systemd/system/iot-rule-engine.service

    systemctl daemon-reload
    systemctl enable iot-rule-engine
    systemctl restart iot-rule-engine

    echo -e "  ${GREEN}✓ 后端服务已安装${NC}"
    echo ""
}

# ─── 检查 Docker 镜像加速器 ───────────────────────────────────────
check_docker_mirror() {
    if ! command -v docker &>/dev/null; then
        echo -e "${RED}错误: 未找到 docker${NC}"
        exit 1
    fi

    # 测试能否拉取 nginx 镜像
    echo "  测试 Docker 镜像拉取..."
    if docker pull --quiet nginx:1.27-alpine 2>/dev/null; then
        echo "  ✓ Docker 拉取正常"
        return
    fi

    echo -e "  ${YELLOW}Docker 镜像拉取失败，检查镜像加速器配置...${NC}"

    local daemon_conf="/etc/docker/daemon.json"
    local need_fix=0

    if [ -f "$daemon_conf" ]; then
        # 检查是否有不可达的 mirror
        if grep -q "docker.1ms.run" "$daemon_conf" 2>/dev/null; then
            echo -e "  ${YELLOW}检测到不可达的镜像加速器: docker.1ms.run${NC}"
            need_fix=1
        fi
    fi

    if [ "$need_fix" -eq 1 ]; then
        # 尝试使用可用加速器
        local mirrors=(
            "https://docker.m.daocloud.io"
            "https://dockerproxy.com"
            "https://docker.nju.edu.cn"
        )

        for mirror in "${mirrors[@]}"; do
            echo "  尝试加速器: $mirror"
            if curl -s --connect-timeout 5 "$mirror/v2/" >/dev/null 2>&1; then
                echo -e "  ${GREEN}  ✓ 可用: $mirror${NC}"
                # 备份原配置
                cp "$daemon_conf" "${daemon_conf}.bak"
                # 更新镜像加速器
                cat > "$daemon_conf" <<EOF
{
  "registry-mirrors": ["$mirror"]
}
EOF
                systemctl daemon-reload
                systemctl restart docker
                echo "  ✓ Docker 已重启，使用新镜像加速器"
                return
            else
                echo "  ✗ 不可用"
            fi
        done

        echo -e "${RED}错误: 所有镜像加速器均不可用，请手动修复 Docker 网络配置${NC}"
        echo "  恢复备份: cp ${daemon_conf}.bak $daemon_conf && systemctl restart docker"
        exit 1
    fi

    echo -e "${RED}错误: Docker 镜像拉取失败，请检查网络连接和 Docker 配置${NC}"
    exit 1
}

# ─── 安装前端 ─────────────────────────────────────────────────────
install_frontend() {
    echo -e "${YELLOW}安装前端...${NC}"

    local frontend_dir="$INSTALL_DIR/frontend"

    echo "  使用 Docker Compose 部署..."

    # 替换端口和代理配置
    sed -i "s/__FRONTEND_PORT__/$FRONTEND_PORT/g" "$frontend_dir/docker-compose.yml" 2>/dev/null || true
    sed -i "s/__BACKEND_HOST__/$BACKEND_HOST/g" "$frontend_dir/docker-compose.yml" 2>/dev/null || true
    sed -i "s/__BACKEND_PORT__/$BACKEND_PORT/g" "$frontend_dir/docker-compose.yml" 2>/dev/null || true

    cd "$frontend_dir"
    docker compose up -d --build
    cd "$INSTALL_DIR"

    echo -e "  ${GREEN}✓ 前端 Docker 容器已启动${NC}"
    echo ""
}

# ─── 检查状态 ─────────────────────────────────────────────────────
check_status() {
    echo -e "${YELLOW}检查服务状态...${NC}"

    echo "  后端服务:"
    systemctl status iot-rule-engine --no-pager --full 2>&1 | head -5 || true

    echo ""

    if command -v docker &>/dev/null; then
        echo "  Docker 容器:"
        docker ps --filter name=iot-rule-engine --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
    fi
}

# ─── 打印摘要 ──────────────────────────────────────────────────────
print_summary() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}IoT Rule Engine 安装完成${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "  安装目录: $INSTALL_DIR"
    echo "  后端服务: iot-rule-engine (systemd)"
    echo "  后端端口: $BACKEND_PORT"
    echo "  前端端口: $FRONTEND_PORT"
    echo ""
    echo "  管理命令:"
    echo "    查看后端状态: systemctl status iot-rule-engine"
    echo "    查看后端日志: journalctl -u iot-rule-engine -f"
    echo "    重启后端:    systemctl restart iot-rule-engine"
    echo "    重启前端:    cd $INSTALL_DIR/frontend && docker compose restart"
    echo ""
    echo "  访问地址: http://<服务器IP>:$FRONTEND_PORT"
    echo ""
}

# ═══════════════════════════════════════════════════════════════════
# 主流程
# ═══════════════════════════════════════════════════════════════════

main() {
    parse_args "$@"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}IoT Rule Engine 一键部署${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    check_requirements
    install_backend
    install_frontend
    check_status
    print_summary
}

main "$@"
