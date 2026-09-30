# IoT Rule Engine 部署说明

`deploy/` 提供通用构建与远端部署模板，不包含任何生产服务器地址或凭证。

## 构建

在项目根目录执行：

```bash
./deploy/build.sh frontend
./deploy/build.sh backend
./deploy/build.sh plugins
./deploy/build.sh all
```

构建结果位于 `deploy/frontend` 和 `deploy/backend`。这些目录中的构建产物、插件二进制、
数据库与日志不应提交到 Git。

## SSH 密钥部署

远端部署只支持 SSH 密钥认证。先复制模板并填写非敏感目标参数：

```bash
cp deploy/deploy-to-server.env.tpl deploy/deploy-to-server.env
```

`deploy-to-server.env` 已被忽略，不能提交。至少需要设置：

```dotenv
REMOTE_HOST=example.internal
REMOTE_PORT=22
REMOTE_USER=deploy
REMOTE_ROOT=/opt/iot-rule-engine
```

执行部署：

```bash
./deploy/deploy-to-server.sh frontend
./deploy/deploy-to-server.sh backend
./deploy/deploy-to-server.sh plugins
./deploy/deploy-to-server.sh all
```

复用已有构建产物：

```bash
SKIP_BUILD=1 ./deploy/deploy-to-server.sh all
```

## 前提条件

- 本机安装 Go、Node.js、npm、OpenSSH 和 rsync；
- 远端安装 Docker、Docker Compose 和 systemd；
- 部署用户通过 SSH Key 登录，并被明确授予所需的服务管理权限；
- 首次连接时核验服务器 Host Key；
- 密码、Token、私钥和生产地址通过外部 Secret Manager 或部署环境注入。

## 默认端口

- Manager API：`11011`；
- 前端部署端口：`3101`。

生产环境应通过 HTTPS 反向代理使用同源 `/api/v1`，不要把 Manager API 直接暴露到公网。
