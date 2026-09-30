# IoT Rule Engine UI

这是 `iot-rule-engine` 同仓维护的控制台，基于 React、TypeScript、Ant Design 和 React Flow，提供规则编排、运行态监控、插件及 Connector 管理、用户和系统配置入口。

## 功能

- 可视化规则编辑、校验、部署和 DryRun
- 规则生命周期、快照和灰度发布管理
- 运行实例、DAG、节点、边和指标监控
- 插件、Connector 资源、用户和系统配置管理

## 本地开发

```bash
cd ui
npm ci
npm start
```

开发模式默认访问 `http://localhost:11011/api/v1`。如需覆盖：

```bash
REACT_APP_API_URL=https://example.com/api/v1 npm start
```

跨端口开发时需把 `http://localhost:3000` 加入引擎的 `security.manager.allowed_origins`。

## 验证与构建

```bash
CI=true npm test -- --runInBand
npm run build
```

生产构建默认使用同源 `/api/v1`，由 `deploy/build.sh` 生成的 Nginx 配置代理到 Manager。
Manager 监听端口是启动项，系统设置页只读展示；修改部署端口时，开发 UI 需要同步修改
`REACT_APP_API_URL`，生产 UI 则修改反向代理目标。未来由引擎内嵌并同源提供 UI 后，会自然
随 Manager 端口一起迁移。

不要把 API Token、管理员密码、NATS 密码或私钥写入 `.env`、源码或浏览器持久配置。敏感配置由引擎部署环境变量注入。
