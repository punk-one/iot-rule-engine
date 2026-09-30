#!/bin/bash

# IoT Rule Engine 部署脚本
# 编译前端和后端，并将结果打包到 frontend 和 backend 目录
# 可以直接拷贝这两个目录到其他 Linux 服务器上部署

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 获取脚本所在目录（deploy 目录）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOYMENTS_DIR="$SCRIPT_DIR"

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}IoT Rule Engine 部署脚本${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# 检查必要的工具
check_requirements() {
    echo -e "${YELLOW}检查环境依赖...${NC}"

    # 检查 Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}错误: 未找到 Go，请先安装 Go 1.24+${NC}"
        exit 1
    fi
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "  ✓ Go 版本: $GO_VERSION"

    # 检查 Node.js
    if ! command -v node &> /dev/null; then
        echo -e "${RED}错误: 未找到 Node.js，请先安装 Node.js 18+${NC}"
        exit 1
    fi
    NODE_VERSION=$(node --version)
    echo "  ✓ Node.js 版本: $NODE_VERSION"

    # 检查 npm
    if ! command -v npm &> /dev/null; then
        echo -e "${RED}错误: 未找到 npm${NC}"
        exit 1
    fi
    NPM_VERSION=$(npm --version)
    echo "  ✓ npm 版本: $NPM_VERSION"

    echo ""
}

# 清理旧的构建产物
clean_build() {
    echo -e "${YELLOW}清理旧的构建产物...${NC}"

    # 清理前端构建目录
    if [ -d "$PROJECT_ROOT/ui/build" ]; then
        rm -rf "$PROJECT_ROOT/ui/build"
        echo "  ✓ 清理前端构建目录"
    fi

    # 清理后端二进制文件
    if [ -f "$PROJECT_ROOT/bin/iot-rule-engine" ]; then
        rm -f "$PROJECT_ROOT/bin/iot-rule-engine"
        echo "  ✓ 清理后端二进制文件"
    fi

    # 清理部署目录中的构建产物
    if [ -d "$DEPLOYMENTS_DIR/frontend/build" ]; then
        rm -rf "$DEPLOYMENTS_DIR/frontend/build"
        echo "  ✓ 清理前端部署目录中的构建产物"
    fi

    if [ -f "$DEPLOYMENTS_DIR/backend/iot-rule-engine" ]; then
        rm -f "$DEPLOYMENTS_DIR/backend/iot-rule-engine"
        echo "  ✓ 清理后端部署目录中的二进制文件"
    fi

    echo ""
}

# 构建前端
build_frontend() {
    echo -e "${YELLOW}构建前端...${NC}"

    cd "$PROJECT_ROOT/ui"

    # 检查 package.json
    if [ ! -f "package.json" ]; then
        echo -e "${RED}错误: 未找到 ui/package.json${NC}"
        exit 1
    fi

    # 安装依赖（如果需要）
    if [ ! -d "node_modules" ]; then
        echo "  安装前端依赖..."
        npm install --legacy-peer-deps
    fi

    # 构建前端
    echo "  构建 React 应用..."
    REACT_APP_API_URL=${REACT_APP_API_URL:-http://localhost:8081/api/v1} npm run build

    if [ ! -d "build" ]; then
        echo -e "${RED}错误: 前端构建失败，未找到 build 目录${NC}"
        exit 1
    fi

    echo -e "  ${GREEN}✓ 前端构建完成${NC}"
    echo ""
}

# 构建后端
build_backend() {
    echo -e "${YELLOW}构建后端...${NC}"

    cd "$PROJECT_ROOT"

    # 检查 go.mod
    if [ ! -f "go.mod" ]; then
        echo -e "${RED}错误: 未找到 go.mod${NC}"
        exit 1
    fi

    # 下载依赖
    echo "  下载 Go 依赖..."
    go mod download

    # 创建 bin 目录
    mkdir -p bin

    # 构建后端
    echo "  编译 Go 应用..."
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o bin/iot-rule-engine ./cmd/main.go

    if [ ! -f "bin/iot-rule-engine" ]; then
        echo -e "${RED}错误: 后端构建失败，未找到二进制文件${NC}"
        exit 1
    fi

    echo -e "  ${GREEN}✓ 后端构建完成${NC}"
    echo ""
}

# 打包前端部署文件
package_frontend() {
    echo -e "${YELLOW}打包前端部署文件...${NC}"

    FRONTEND_DIR="$DEPLOYMENTS_DIR/frontend"

    # 创建必要的目录
    mkdir -p "$FRONTEND_DIR/build"

    # 复制构建产物
    echo "  复制前端构建产物..."
    cp -r "$PROJECT_ROOT/ui/build"/* "$FRONTEND_DIR/build/"

    # 复制配置文件（如果不存在）
    if [ ! -f "$FRONTEND_DIR/nginx.conf" ]; then
        echo "  复制 nginx.conf..."
        # nginx.conf 应该已经存在，这里只是检查
    fi

    # 创建部署用的 Dockerfile（使用本地构建产物）
    echo "  创建部署用 Dockerfile..."
    cat > "$FRONTEND_DIR/Dockerfile.deploy" << 'EOF'
# 前端部署 Dockerfile（使用本地构建产物）
FROM nginx:alpine

# 复制构建产物
COPY build /usr/share/nginx/html

# 复制 nginx 配置文件
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 暴露端口
EXPOSE 80

# 启动 nginx
CMD ["nginx", "-g", "daemon off;"]
EOF

    # 创建部署用的 docker-compose.yml
    echo "  创建部署用 docker-compose.yml..."
    cat > "$FRONTEND_DIR/docker-compose.deploy.yml" << 'EOF'
version: '3.8'

services:
  frontend:
    build:
      context: .
      dockerfile: Dockerfile.deploy
    container_name: iot-rule-engine-frontend
    restart: unless-stopped
    ports:
      - "3000:80"
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost/"]
      interval: 30s
      timeout: 10s
      retries: 3
EOF

    # 创建部署说明文件
    cat > "$FRONTEND_DIR/DEPLOY.md" << 'EOF'
# 前端部署说明

## 快速部署

1. 确保已安装 Docker 和 Docker Compose

2. 使用部署用 docker-compose 文件启动：
```bash
docker-compose -f docker-compose.deploy.yml up -d
```

3. 或者使用 Dockerfile 直接构建：
```bash
docker build -f Dockerfile.deploy -t iot-rule-engine-frontend:latest .
docker run -d -p 3000:80 --name punk-frontend iot-rule-engine-frontend:latest
```

## 文件说明

- `build/`: 前端构建产物（React 应用）
- `Dockerfile.deploy`: 部署用 Dockerfile（使用本地构建产物）
- `docker-compose.deploy.yml`: 部署用 docker-compose 配置
- `nginx.conf`: Nginx 配置文件
- `README.md`: 详细部署文档

## 注意事项

- 前端需要知道后端 API 地址，默认是 `http://localhost:8081/api/v1`
- 如果后端地址不同，需要重新构建前端（修改 REACT_APP_API_URL 环境变量）
EOF

    echo -e "  ${GREEN}✓ 前端部署文件打包完成${NC}"
    echo ""
}

# 打包后端部署文件
package_backend() {
    echo -e "${YELLOW}打包后端部署文件...${NC}"

    BACKEND_DIR="$DEPLOYMENTS_DIR/backend"

    # 创建必要的目录
    mkdir -p "$BACKEND_DIR"
    mkdir -p "$BACKEND_DIR/configs"

    # 复制二进制文件
    echo "  复制后端二进制文件..."
    cp "$PROJECT_ROOT/bin/iot-rule-engine" "$BACKEND_DIR/"
    chmod +x "$BACKEND_DIR/iot-rule-engine"

    # 复制配置文件
    echo "  复制配置文件..."
    if [ -d "$PROJECT_ROOT/configs" ]; then
        cp -r "$PROJECT_ROOT/configs"/* "$BACKEND_DIR/configs/"
    else
        echo -e "  ${YELLOW}警告: 未找到 configs 目录${NC}"
    fi

    # 创建部署用的 Dockerfile（使用本地构建产物）
    echo "  创建部署用 Dockerfile..."
    cat > "$BACKEND_DIR/Dockerfile.deploy" << 'EOF'
# 后端部署 Dockerfile（使用本地构建产物）
FROM alpine:latest

# 安装必要的运行时依赖
RUN apk --no-cache add ca-certificates tzdata wget

# 设置时区
ENV TZ=Asia/Shanghai
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

WORKDIR /app

# 复制二进制文件
COPY bin/iot-rule-engine /app/iot-rule-engine

# 复制配置文件
COPY configs /app/configs

# 创建数据目录
RUN mkdir -p /app/data /app/plugins

# 暴露端口
EXPOSE 8081 8087 1883 8083 9090 9091

# 设置环境变量
ENV CONFIG_PATH=/app/configs/config.yaml

# 运行应用
CMD ["/app/iot-rule-engine", "-config", "/app/configs/config.yaml"]
EOF

    # 创建部署用的 docker-compose.yml
    echo "  创建部署用 docker-compose.yml..."
    cat > "$BACKEND_DIR/docker-compose.deploy.yml" << 'EOF'
version: '3.8'

services:
  backend:
    build:
      context: .
      dockerfile: Dockerfile.deploy
    container_name: iot-rule-engine-backend
    restart: unless-stopped
    ports:
      - "8081:8081"  # Manager API
      - "8087:8087"  # HTTP Ingress
      - "1883:1883"  # MQTT TCP
      - "8083:8083"  # MQTT WebSocket
      - "9090:9090"  # Executor API
      - "9091:9091"  # Pipeline API
    volumes:
      - backend-data:/app/data
      - backend-plugins:/app/plugins
      - ./config.yaml:/app/configs/config.yaml:ro
    environment:
      - CONFIG_PATH=/app/configs/config.yaml
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8081/api/v1/health"] || exit 1
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

volumes:
  backend-data:
    driver: local
  backend-plugins:
    driver: local
EOF

    # 创建部署说明文件
    cat > "$BACKEND_DIR/DEPLOY.md" << 'EOF'
# 后端部署说明

## 快速部署

1. 确保已安装 Docker 和 Docker Compose

2. 使用部署用 docker-compose 文件启动：
```bash
docker-compose -f docker-compose.deploy.yml up -d
```

3. 或者使用 Dockerfile 直接构建：
```bash
docker build -f Dockerfile.deploy -t iot-rule-engine-backend:latest .
docker run -d \
  -p 8081:8081 -p 8087:8087 -p 1883:1883 -p 8083:8083 \
  -p 9090:9090 -p 9091:9091 \
  -v punk-backend-data:/app/data \
  -v punk-backend-plugins:/app/plugins \
  --name punk-backend \
  iot-rule-engine-backend:latest
```

## 文件说明

- `iot-rule-engine`: 后端二进制文件
- `configs/`: 配置文件目录
- `config.yaml`: 部署配置文件（会覆盖 configs 中的默认配置）
- `Dockerfile.deploy`: 部署用 Dockerfile（使用本地构建产物）
- `docker-compose.deploy.yml`: 部署用 docker-compose 配置
- `README.md`: 详细部署文档

## 注意事项

- 数据会持久化到 Docker 卷中（backend-data 和 backend-plugins）
- 需要确保 plugins 目录中有编译好的插件文件（.so 文件）
- 可以通过修改 config.yaml 来自定义配置
EOF

    echo -e "  ${GREEN}✓ 后端部署文件打包完成${NC}"
    echo ""
}

# 主函数
main() {
    echo "项目根目录: $PROJECT_ROOT"
    echo "部署目录: $DEPLOYMENTS_DIR"
    echo ""

    # 检查环境
    check_requirements

    # 清理旧构建
    clean_build

    # 构建前端
    build_frontend

    # 构建后端
    build_backend

    # 打包前端
    package_frontend

    # 打包后端
    package_backend

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}部署文件准备完成！${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "前端部署目录: $DEPLOYMENTS_DIR/frontend"
    echo "  包含文件:"
    echo "    - build/ (前端构建产物)"
    echo "    - Dockerfile.deploy"
    echo "    - docker-compose.deploy.yml"
    echo "    - nginx.conf"
    echo "    - DEPLOY.md"
    echo ""
    echo "后端部署目录: $DEPLOYMENTS_DIR/backend"
    echo "  包含文件:"
    echo "    - iot-rule-engine (后端二进制)"
    echo "    - configs/ (配置文件)"
    echo "    - config.yaml"
    echo "    - Dockerfile.deploy"
    echo "    - docker-compose.deploy.yml"
    echo "    - DEPLOY.md"
    echo ""
    echo -e "${YELLOW}现在可以将 frontend 和 backend 目录拷贝到目标服务器进行部署${NC}"
    echo ""
}

# 运行主函数
main
