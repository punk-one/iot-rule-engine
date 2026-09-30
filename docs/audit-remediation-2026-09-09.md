# IoT Rule Engine 审计整改记录（2026-09-09）

本文跟踪 `comprehensive-audit-2026-09-09.md` 中发现的问题。原报告保留为整改前证据，本文件记录已实施代码、验证结果和仍需外部动作的发布门槛。

## 结果

| 编号 | 状态 | 本轮处理 |
| --- | --- | --- |
| ENG-A01 | 已完成 | `sys_users` 增加可迁移的 `auth_version`；改密和启停用户原子推进代际并撤销该用户全部内存会话；每次 token 校验同时核对数据库状态与代际，关闭并发登录/禁用窗口。 |
| ENG-A02 | 已完成 | 去重改为并发安全的 reserve/commit/rollback；只有 Scheduler 接受任务才提交标记，失败分支回滚；并发相同事件等待首个提交结果；批量路径按边分别提交或回滚。 |
| ENG-A03 | 已完成（当前范围） | 新增统一 `NormalizeAndValidate`，启动加载和配置 API 共用；Manager/数据库/Redis/NATS/投递/背压等关键互斥和范围错误在保存前返回，不再把无效配置留到重启才发现。修订历史和自动回滚仍可作为后续增强。 |
| ENG-A04 | 已完成 | 配置整体保存前从当前持久化配置合并数据库、Redis、NATS 和 Manager 隐藏秘密；GET/PUT 不再静默清空这些值。显式清除秘密仍应单独设计 API。 |
| ENG-A05 | 部分完成 | bcrypt 校验并发限制为 4；每用户最多 10 个会话、进程最多 10000 个；每分钟清理过期会话，创建会话时也清理并逐出最旧记录。账号+来源地址限速仍待接入 HTTP 层。 |
| ENG-A06 | 已完成 | 证书上传总量限制 2 MiB；只接受可解析的 X.509/PKCS 密钥 PEM；服务端随机命名、拒绝覆盖；目录 `0700`、文件 `0600`，写失败自动删除半文件。 |
| ENG-A07 | 外部发布门槛 | Git 历史凭证路径仍需在公开推送前完成轮换、全历史扫描和授权清理；代码修改无法替代凭证轮换。 |

## 复现闭环

整改前的隔离复现程序现输出：

```text
old_token_accepted_after_password_reset=false
old_token_accepted_after_disable=false
fresh_login_blocked_after_disable=true

fill_ok=true
first_attempt_rejected=true
retry_falsely_accepted=false
```

SQLite 旧表缺少 `auth_version` 时会在启动时自动增加该列，已有管理员和用户不需要重建。

## 验证

```text
go test -race -count=1 ./...  PASS
go vet ./...                  PASS
```

新增测试覆盖：改密/重置/禁用后的旧 token、旧 SQLite schema 迁移、会话容量与过期回收、去重回滚与并发预约、满队列重试、配置语义校验、隐藏秘密保留和 PEM 校验。

## 仍需完成的发布验收

- 账号+来源地址登录限速，以及限速命中审计指标。
- 配置修订号、last-known-good 和显式回滚操作。
- 授权设备环境中的 S7/MQTT 故障注入、24/72 小时长稳和 1000 次部署/取消部署资源回落。
- 公开 Git 历史的凭证扫描、轮换记录和干净发布基线。
