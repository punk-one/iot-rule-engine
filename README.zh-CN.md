# IoT Rule Engine

[English](README.md) | [简体中文](README.zh-CN.md)

IoT Rule Engine 是一个基于 Go 的 IoT 规则引擎，当前代码形态包含四类运行面。

本项目采用三仓协作结构：

- 引擎：`github.com/punk-one/iot-rule-engine`
- SDK：`github.com/punk-one/iot-rule-plugin-sdk`
- 官方插件：`github.com/punk-one/iot-rule-plugins`

引擎仓只维护引擎、管理 UI、插件运行时和测试夹具，正式插件源码由插件仓维护。

插件运行时采用“每个 artifact 一个 OS 进程、每个规则节点一个进程内对象”的模型。
无论多少规则引用同一个 `.so`，引擎只启动一个该插件进程；规则之间的配置、Context、
事件和 Stop 生命周期仍完全隔离。Connector 同样每个插件类型只保留一个进程，资源
通过 `provider_handle` 隔离。

当前项目版本：`v1.0.0`

- `Ingress`：协议入口，负责接收 HTTP / MQTT 数据。
- `Manager`：控制面与运维面，统一暴露规则、插件、配置、用户和运行态 API。
- `Executor`：规则执行面，负责装载规则 DAG、运行插件、维护运行实例。
- `Pipeline`：规则路由面，按规则 source topic 将输入事件路由到对应规则。

当前版本已经完成端口收敛：

- Manager 对外统一监听 `manager.port`，默认 `11011`
- Executor 不再单独暴露 HTTP 端口
- Pipeline 不再单独暴露 HTTP 端口
- Runtime、Executor 运维、Pipeline 运维入口全部合并到 Manager API

## 架构概览

```text
HTTP / MQTT Clients
        |
        v
     Ingress
        |
        v
   Event Bridge / NATS
        |
        v
     Pipeline
        |
        v
  rule.input.{ruleID}
        |
        v
     Executor
        |
        v
Plugins / Runtime / DLQ

Manager API <------ shared RuntimeManager / RulesDB / Config
```

关键代码位置：

- 进程入口：`cmd/main.go`
- 顶层生命周期：`internal/server/engine.go`
- 配置加载：`internal/infra/config/loader.go`
- Manager 路由：`internal/server/manager/router.go`
- Executor 启动：`internal/server/executor/executor.go`
- Pipeline 启动：`internal/infra/pipeline/server.go`
- HTTP Ingress：`internal/server/ingress/http/http_ingress.go`
- MQTT Ingress：`internal/server/ingress/mqtt/mqtt_ingress.go`

## 服务与端口

默认配置见 `configs/config.yaml`。

| 组件 | 默认值 | 说明 |
| --- | --- | --- |
| Manager API | `http://localhost:11011/api/v1` | 控制面与运维面统一入口 |
| HTTP Ingress | `http://localhost:8087` | `POST /ingress/http`，健康检查 `GET /health` |
| MQTT Broker TCP | `localhost:1883` | MQTT 接入 |
| MQTT Broker WS | `ws://localhost:8083` | MQTT over WebSocket |

Manager 合并后的运维路径：

- `GET /api/v1/health`
- `GET /api/v1/metrics`
- `GET /api/v1/executor/health`
- `GET /api/v1/executor/vars`
- `GET /api/v1/executor/stats`
- `GET /api/v1/pipeline/health`
- `GET /api/v1/pipeline/metrics`
- `GET /api/v1/runtime/*`

## 配置与存储

配置结构定义位于：

- `internal/infra/config/config.go`
- `internal/infra/config/services.go`
- `internal/infra/config/security.go`
- `internal/infra/config/storage.go`

当前支持的核心配置：

- `manager.storage_type`: `sqlite` 或 `postgres`
- `manager.db_path`: 控制面数据目录，SQLite 模式下默认保存 `rules.db`
- `manager.init_db`: PostgreSQL 模式下首次启动时，是否用当前 YAML 初始化 `re_config.system`
- `executor.state_backend.type`: `memory` 或 `redis`
- `common.nats.*`: NATS 连接与认证参数
- `security.*`: Plugin、Dispatcher、Ingress、Manager 安全限制

配置加载优先级：

1. 先加载 `configs/config.yaml`
2. 如果控制面数据库中存在已保存配置，则用数据库配置覆盖
3. 最后用环境变量覆盖密码、Token 和证书路径等秘密字段

这意味着：

- `configs/config.yaml` 是默认值和冷启动模板
- 配置管理页面保存后的内容写入数据库
- 修改数据库中的配置后需要重启服务生效

## 认证与用户

Manager 当前使用用户会话认证，登录后通过 Bearer Token 访问控制面 API。

认证相关接口：

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `PUT /api/v1/auth/password`
- `GET /api/v1/users`
- `POST /api/v1/users`
- `PUT /api/v1/users/:id/password`
- `PUT /api/v1/users/:id/status`

系统启动时会自动确保管理员账号存在。首次使用空数据库启动前必须设置
`IOT_RULE_ADMIN_PASSWORD`；密码须为 12–72 字节，并同时包含大写字母、小写字母、
数字和符号。数据库已有管理员时无需再次提供该变量。

## 快速开始

### 1. 准备依赖

- Go
- Node.js 与 npm（仅前端开发需要）

### 2. 检查默认配置

编辑 `configs/config.yaml`，至少确认以下项目：

- `common.nats.mode`（默认 `internal`，无需另行部署 NATS）
- `manager.storage_type`
- `manager.db_path`
- `manager.init_db`
- `ingress.http.port`
- `ingress.mqtt.tcp_port`
- `manager.port`

最小可运行配置示例：

```yaml
common:
  nats:
    mode: "internal"
    host: "127.0.0.1"
    port: 0
    store_dir: "./data/nats"
    url: ""
    auth_mode: "none"
    max_reconnects: -1
    reconnect_wait_ms: 2000

ingress:
  http:
    enabled: true
    port: 8087
    auth_enabled: true
    token: ""
  mqtt:
    enabled: true
    tcp_port: 1883
    ws_port: 8083
    auth_mode: "anonymous"
    users: []

manager:
  port: 11011
  storage_type: "sqlite"
  db_path: "./data"

executor:
  plugins_dir: "./plugins"
  worker_count: 1
  state_backend:
    type: "memory"

security:
  manager:
    token: ""
    auth_enabled: true
    allow_insecure: false
    allowed_origins:
      - "http://localhost:3000"
      - "http://127.0.0.1:3000"
    allow_runtime_plugin_install: false
    trusted_plugin_manifest: ""
    max_plugin_upload_mb: 64
```

说明：

- `common.nats.mode: internal` 会随引擎启动内置 JetStream；`port: 0` 会选择随机可用端口，实际地址自动传给插件
- 使用独立 NATS 时设置 `common.nats.mode: external` 并填写 `url`；认证字段只用于外部模式
- NATS 凭证不得提交到仓库，外部模式应通过部署密钥挂载或 secret manager 提供
- 首次启动前应通过部署 secret 设置 `IOT_RULE_ADMIN_PASSWORD`，不要把管理员密码写入 YAML
- 同源请求自动允许；`allowed_origins` 用于额外允许跨域前端。默认值面向本地开发，生产环境应替换为实际 HTTPS 管理端来源
- 在线插件安装默认关闭；生产环境建议离线发布。确需启用时，必须提供已在引擎外完成签名验证且只读挂载的 release manifest
- 当前配置里已经没有 `pipeline` 段；Pipeline 运维接口已合并到 Manager
- 如果启用 PostgreSQL，把 `manager.storage_type` 改成 `postgres`，并补全 `database` 段
- PostgreSQL 首次初始化配置时，可设置 `manager.init_db: true`，启动时会在 `re_config` 缺少 `system` 记录时用当前 YAML 写入一次，不会覆盖数据库里已有配置
- 如果启用 Redis 状态后端，把 `executor.state_backend.type` 改成 `redis`，并补全 `redis` 段

### 3. 启动后端

```bash
export IOT_RULE_ADMIN_PASSWORD='Replace-With-A-Strong1!'
go run ./cmd/main.go
```

可通过以下环境变量注入秘密值，并覆盖 YAML/数据库中的对应字段：

- `IOT_RULE_DATABASE_PASSWORD`
- `IOT_RULE_REDIS_PASSWORD`
- `IOT_RULE_NATS_USERNAME`、`IOT_RULE_NATS_PASSWORD`、`IOT_RULE_NATS_TOKEN`
- `IOT_RULE_NATS_CREDS_FILE`、`IOT_RULE_NATS_CERT_FILE`、`IOT_RULE_NATS_KEY_FILE`、`IOT_RULE_NATS_CA_FILE`
- `IOT_RULE_MANAGER_TOKEN`

启动后包含：

- HTTP Ingress
- MQTT Ingress
- Manager API
- Executor
- Pipeline

### 4. 启动前端

```bash
cd ui
npm install
npm start
```

前端默认访问：

- `http://localhost:3000`

部署脚本默认把前端发布到：

- `http://<server>:3101`

前端请求模式：

- `development` 默认直连 `http://localhost:11011/api/v1`
- `production` 默认走同源 `/api/v1`

`manager.port` 是启动监听项，不能通过当前 Manager 连接安全地在线切换。系统设置页面仅展示该值：
开发模式通过 `REACT_APP_API_URL` 指定目标；生产模式使用同源 `/api/v1`，由反向代理或未来的
内嵌 UI 服务与 Manager 端口保持一致。

如需覆盖，可设置环境变量：

- `REACT_APP_API_URL`

## 常用 API

### 规则管理

- `GET /api/v1/rules`
- `GET /api/v1/rules?full=true`
- `GET /api/v1/rules/:id`
- `POST /api/v1/rules`
- `PUT /api/v1/rules/:id`
- `DELETE /api/v1/rules/:id`
- `POST /api/v1/rules/:id/deploy`
- `POST /api/v1/rules/:id/enable`
- `POST /api/v1/rules/:id/disable`
- `POST /api/v1/rules/validate`
- `POST /api/v1/rules/dry-run`

### 快照与灰度发布

- `GET /api/v1/rules/:id/snapshots`
- `GET /api/v1/rules/:id/snapshots/latest`
- `GET /api/v1/rules/:id/snapshots/:version`
- `POST /api/v1/rules/:id/snapshots`
- `GET /api/v1/rules/:id/gray-release`
- `POST /api/v1/rules/:id/gray-release`
- `PUT /api/v1/rules/:id/gray-release`
- `DELETE /api/v1/rules/:id/gray-release`

### 插件管理

- `GET /api/v1/plugins`
- `GET /api/v1/plugins?descriptors=true`
- `GET /api/v1/plugins/:name`
- `POST /api/v1/plugins/upload`
- `POST /api/v1/plugins/:name/enable`
- `POST /api/v1/plugins/:name/disable`
- `POST /api/v1/plugins/:name/upgrade`
- `DELETE /api/v1/plugins/:name`

### 配置管理

- `GET /api/v1/config`
- `PUT /api/v1/config`
- `POST /api/v1/config/upload`

### 运行态

当前运行态使用 `instance_id` 作为主查询键。

- `GET /api/v1/runtime/instances`
- `GET /api/v1/runtime/instances/:instance_id`
- `GET /api/v1/runtime/rules/:rule_id/instance`
- `GET /api/v1/runtime/instances/:instance_id/nodes`
- `GET /api/v1/runtime/instances/:instance_id/nodes/:node_id`
- `GET /api/v1/runtime/instances/:instance_id/edges`
- `GET /api/v1/runtime/instances/:instance_id/edges/:edge_id`
- `GET /api/v1/runtime/instances/:instance_id/trace`
- `GET /api/v1/runtime/instances/:instance_id/metrics`
- `GET /api/v1/runtime/dlq`
- `POST /api/v1/runtime/dlq/replay`

## 项目结构

```text
cmd/                    主程序入口
configs/                默认配置
docs/                   项目文档
internal/
  core/                 引擎、调度、运行态、插件核心
  infra/                bus、config、pipeline、storage
  server/               manager、ingress、executor 运行面
plugins/                插件二进制目录
tools/                  工具程序与脚本
ui/                     React 前端
```

## 文档

- `docs/README.md`：文档索引
- `docs/audit-remediation-2026-09-05.md`：本轮可靠性、安全与扩展性整改结果
- `docs/security-deployment.md`：生产部署安全基线和插件信任边界
- `docs/架构与系统总览.md`：系统架构、组件边界、关键文件
- `docs/运行时与调度.md`：Ingress、Pipeline、Executor、Runtime、DLQ、部署流程
- `docs/UI与运维规范.md`：前端页面与运维 API 对照
- `docs/插件与SDK规范.md`：插件 SDK 与 import 约束
- `docs/数据存储与处理.md`：平台存储边界说明
- `docs/目录结构规范.md`：目录职责约定

## 说明

- 历史上仓库内曾存在多份设计稿、升级建议和阶段性说明；当前文档已按“代码现状优先”收敛。
- 如果文档与代码冲突，以 `internal/` 和 `ui/src/` 中的当前实现为准，并应回写文档。

## 许可证

IoT Rule Engine 采用 [GNU Affero General Public License v3.0 only](LICENSE)
（`AGPL-3.0-only`）。如果修改本软件并通过网络向用户提供该修改版本，应在部署前确认
AGPL 第 13 条规定的对应源代码提供义务。AGPL 允许商业使用，但不授予项目名称、Logo
或其他商标的使用权。
