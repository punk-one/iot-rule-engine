# 生产部署安全基线

## 默认策略

- Manager 默认必须鉴权；只有显式设置 `allow_insecure: true` 才允许关闭。
- 首次空库启动必须由 `IOT_RULE_ADMIN_PASSWORD` 提供强管理员密码。
- NATS 默认以内置模式运行，监听 loopback，端口 `0` 表示随机可用端口，不使用外部凭证。
- 在线插件安装默认关闭。生产环境优先使用离线、不可变制品部署。
- pprof 默认关闭；开启后仍需管理员认证。
- `allowed_origins` 只扩展允许跨域来源；Manager 自身的同源请求始终允许。仓库默认配置面向本地开发，生产部署必须替换为实际 HTTPS 控制台来源。

## Secret 注入

不要把密码、Token、NATS creds 或 TLS 私钥写入仓库和配置数据库。使用部署平台 secret 注入下列环境变量或受限文件路径：

```text
IOT_RULE_ADMIN_PASSWORD
IOT_RULE_DATABASE_PASSWORD
IOT_RULE_REDIS_PASSWORD
IOT_RULE_NATS_USERNAME
IOT_RULE_NATS_PASSWORD
IOT_RULE_NATS_TOKEN
IOT_RULE_NATS_CREDS_FILE
IOT_RULE_NATS_CERT_FILE
IOT_RULE_NATS_KEY_FILE
IOT_RULE_NATS_CA_FILE
IOT_RULE_MANAGER_TOKEN
```

环境变量在 YAML 和数据库配置之后应用，因此可用于部署时最终覆盖。

## 在线插件安装的信任边界

在线安装会执行本机二进制，不能仅靠文件扩展名或插件自报身份建立信任。若业务必须启用：

```yaml
security:
  manager:
    auth_enabled: true
    allow_insecure: false
    allowed_origins:
      - "https://rules.example.com"
    allow_runtime_plugin_install: true
    trusted_plugin_manifest: "/etc/iot-rule/trusted/release-manifest.json"
    max_plugin_upload_mb: 64
```

release manifest 必须由发布流水线在引擎外完成签名验证，并以只读、管理员不可通过 API 修改的方式挂载。引擎会在首次执行上传文件之前校验 manifest 中的 artifact SHA-256、ID、版本、类型和精确 SDK 标识 `github.com/punk-one/iot-rule-plugin-sdk@v1.0.0`。

这仍不是操作系统沙箱。生产环境应让插件进程运行在独立低权限用户或容器中，并按需限制网络、文件系统、系统调用、CPU 与内存。

## 发布检查

- 确认仓库和镜像中没有 `.creds`、NKey seed、TLS 私钥或固定密码。
- 轮换任何曾进入 Git 历史的凭证；删除当前文件不等于完成轮换。
- 固定 SDK `v1.0.0` 和插件 artifact digest。
- 执行 `go test ./...`、`go test -race ./...`、`go vet ./...`。
- 在目标环境验证优雅停止、NATS 断线恢复、插件崩溃恢复和升级回滚。
