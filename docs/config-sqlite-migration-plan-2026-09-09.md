# IoT Rule Engine 配置 SQLite 化方案

- 日期：2026-09-09
- 目标：运行配置以 SQLite 为权威来源，支持独立、多页签的系统设置页面，并保留可恢复的启动边界

## 当前状态

后端存在 `configs/config.yaml`。加载顺序为：先读 YAML 得到存储位置和初始值，再打开规则库，
如果 `config` 表存在 `key=system` 则用数据库内容覆盖。`PUT /api/v1/config` 已能把整份配置
保存到数据库，但当前仍存在以下问题：

- SQLite 首次启动原先不会自动建立系统配置记录；
- 配置是一整块 YAML 文本，没有版本、分区、变更审计或并发控制；
- 页面保存后统一提示重启，未区分可热加载字段和必须重启字段；
- API 返回完整配置结构，未来不能把口令和私钥明文回显给浏览器；
- `fallbackConfigPath` 是硬编码，和启动参数指定的配置文件可能不一致。

本轮已完成第一阶段：SQLite 首次启动会把 bootstrap YAML 自动写入 `config.system`，之后
数据库优先；独立 `/config` “系统设置”菜单已恢复，页面使用基础/NATS、服务组件、安全配置
三个页签。用户管理仍只在右上角用户菜单中，不进入系统设置页签。

Manager API 默认端口统一为 `11011`。`manager.port` 属于打开 SQLite 和提供设置页面之前就
必须确定的 bootstrap 项；当前页面只读展示，不能在承载本次请求的连接中在线修改。开发 UI
通过 `REACT_APP_API_URL` 指定后端，生产 UI 默认使用同源 `/api/v1` 并由反向代理指向 Manager。
未来 UI 由同一个可执行文件提供时，浏览器将自然跟随当前 Manager 地址和端口。

## 不能完全删除的启动配置

SQLite 文件的位置不能存放在它自己里面，否则进程启动时无法知道该打开哪个文件；加密 SQLite
配置的主密钥也不能与密文放在同一个库中。因此建议将配置明确拆成两层：

```text
进程参数 / 环境变量（最小 bootstrap）
  ├─ data_dir / sqlite_file
  ├─ listen_address（默认 `:11011`，可选应急覆盖）
  └─ master_key 或 Secret Provider 引用
          ↓ 打开 SQLite
SQLite 系统设置（运行配置权威来源）
  ├─ message_bus
  ├─ executor
  ├─ plugin_security
  ├─ api_security
  ├─ logging
  └─ observability
```

最终 `config.yaml` 应改名为 `bootstrap.yaml`，只保留上述最小字段。容器部署可完全不用文件，
改用 `IOT_RULE_DATA_DIR`、`IOT_RULE_DB_FILE`、`IOT_RULE_MASTER_KEY_FILE`。这不违背“配置保存到
SQLite”，因为 bootstrap 是定位和解密配置库的启动参数，不是运行配置。

## 目标数据模型

不要继续把整份 YAML 作为唯一记录。建议新增：

```sql
CREATE TABLE system_settings (
  namespace   TEXT NOT NULL,
  key         TEXT NOT NULL,
  value_json  TEXT NOT NULL,
  secret_ref  TEXT,
  version     INTEGER NOT NULL,
  apply_mode  TEXT NOT NULL, -- hot_reload | restart_required
  updated_at  TEXT NOT NULL,
  updated_by  TEXT NOT NULL,
  PRIMARY KEY (namespace, key)
);

CREATE TABLE system_setting_revisions (
  revision_id TEXT PRIMARY KEY,
  version     INTEGER NOT NULL,
  snapshot_json TEXT NOT NULL,
  changed_by  TEXT NOT NULL,
  changed_at  TEXT NOT NULL,
  change_note TEXT
);
```

更新使用事务和乐观锁：客户端提交 `base_version`，版本不一致返回 409，避免两个管理员互相
覆盖。保存前先对完整候选配置执行结构、范围、路径和交叉字段校验；写入 revision 后再更新
current。支持按 revision 回滚。

## 页面信息架构

独立菜单名称使用“系统设置”，路由保留 `/config`。建议稳定为五个页签：

| 页签 | 内容 | 默认生效方式 |
| --- | --- | --- |
| 消息总线 | 内置/外部 NATS、随机端口、JetStream 目录、外部认证引用 | 重启 |
| 运行与插件 | worker、错误历史、插件目录、RPC/QPS/Payload、重启与熔断 | 混合 |
| API 与安全 | 鉴权、Origin、上传限制、可信 manifest | 重启 |
| 日志与观测 | level、轮转、pprof、指标采样 | 日志热加载，其余重启 |
| 版本与审计 | 当前版本、待生效字段、历史、差异、回滚 | 即时 |

每个字段旁明确显示“立即生效”或“重启后生效”。保存操作分为“保存草稿”和“保存并应用”；
若包含重启字段，只生成待生效 revision，不允许 Web 进程自行重启宿主服务。

## 密钥与证书

- API 永不返回口令、token、私钥原文，只返回 `configured: true` 和 secret reference；
- 环境变量仍可作为最高优先级应急覆盖，但页面必须显示“被部署环境覆盖”；
- 如必须保存到 SQLite，使用 envelope encryption，主密钥来自文件权限受控的环境 Secret；
- 证书上传使用随机文件名和固定目录，拒绝路径穿越，记录 SHA-256、用途、到期时间；
- 删除证书前检查当前设置和运行资源引用。

## 迁移步骤

1. **已完成**：首次启动将 YAML seed 到 SQLite；数据库优先；恢复独立多页签页面。
2. 新增 settings/revisions 表和 repository，保留旧 `config.system` 只读兼容。
3. 启动时把旧整块配置迁移为 namespace/key，并写 migration revision；过程幂等。
4. API 改为分区读取/校验/乐观锁，敏感字段改 secret reference。
5. 增加热加载协调器；不能热加载的字段标记 pending restart。
6. 将 `config.yaml` 缩减并重命名为 `bootstrap.yaml`，连续两个小版本兼容旧参数。
7. 完成备份、恢复、并发编辑、错误回滚和断电一致性测试后删除旧 blob 写路径。

## 验收标准

- 空数据目录首次启动后，SQLite 中必有可读取的系统配置；第二次启动不再覆盖页面修改；
- 缺失 SQLite 时可由 bootstrap 安全恢复，损坏库必须失败并给出明确恢复指引，不能静默回退；
- 非管理员无法读写系统设置，API 不泄露 secret；
- 两个管理员并发保存时必有一个收到版本冲突，不能最后写入静默覆盖；
- 配置校验失败不产生 current 变更，revision 可审计；
- 热加载失败自动回滚；restart-required 字段清楚显示待生效；
- SQLite 备份恢复后，规则、Connector、用户、系统设置和 revision 一致。
