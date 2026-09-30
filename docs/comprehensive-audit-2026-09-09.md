# IoT Rule Engine 当前实现综合审计（2026-09-09）

## 范围与结论

审计对象为本地当前工作树，HEAD 为 `2dd7b0f61f1ca3b7ca5dfc21d073a941d8e8b9da`，包含大量已暂存、未暂存和未跟踪的迁移实现。HEAD 不能单独重现本报告结果。环境：WSL Ubuntu、Go 1.25.0 linux/amd64、Node 20.19.5。审计日期 2026-09-09。

本次检查调度所有权、背压与去重、用户会话、配置持久化、UI 回归、SDK/插件集成与发布准备。采取关键代码审查、现有自动化测试和独立临时数据库复现。没有修改生产逻辑、线上规则或管理员账户，没有重新部署规则，也没有访问真实 PLC/MQTT 做故障测试。

**结论：开发验证通过，但暂不建议作为生产稳定版发布。** 现有竞态和 UI 测试均通过；本轮仍复现了会话撤销失效和队列拒绝后的重试静默丢失。必须先修复这些可靠性与安全缺陷。报告不代表全量渗透测试、漏洞库扫描或长稳认证。

评级采用可操作结论：可靠性“待整改”、稳定性“有回归基础但存在长时间运行风险”、可扩展性“架构可用，需补齐跨版本与容量验证”。不把通过测试等同于不存在缺陷。

## 本轮验证

| 检查 | 结果与边界 |
| --- | --- |
| `go test -race -count=1 ./...` | 通过，禁用测试缓存；包含本地子进程/隔离集成测试 |
| `go vet ./...` | 通过 |
| UI：`CI=true npm test -- --watchAll=false --runInBand` | 19 套、58 项全部通过 |
| 临时数据库会话复现 | 确认密码重置和用户禁用后旧 token 仍有效 |
| 隔离调度器满队列复现 | 确认未成功入队事件的立即重试返回 nil |
| Git 历史文件检查 | `6bc4bc5` 涉及已删除的 NATS creds 路径；没有读取或复制凭证正文 |
| 构建、线上写入、故障注入 | 本轮未重新执行 UI 生产构建，未对运行服务发起写入或故障注入 |
| 收尾时本地服务存活检查 | 3000/11011 均无监听，HTTP 连接失败，未发现 tmux 会话；本轮未执行启停，无法确定服务停止时间与原因 |

本轮证据程序在 [audit-repros-2026-09-09](audit-repros-2026-09-09/) 中，带 `//go:build ignore`，不会进入正常构建与测试。
为使复现资料随报告保留，.gitignore 仅增加该日期审计目录的例外，其他运行文件忽略规则保持原样。

## 发现与整改建议

### ENG-A01 · P1 · 禁用用户或改密后，已有会话未撤销（已复现）

**证据：** `internal/server/manager/userauth/service.go:185` 的 ValidateToken 只查内存会话与到期时间；`:327` 的 UpdateUserStatus 和 `:572` 的 updatePassword 仅更新数据库，没有撤销会话。当前 sessionTTL 为 24 小时。

用户被禁用后新登录被拒绝，但旧 token 仍可通过通用鉴权；重置密码同样不能终止已泄露令牌的访问权限。

复现：在仓库根目录运行 `go run docs/audit-repros-2026-09-09/session_repro.go`。使用独立临时 SQLite，运行后删除；不会触碰实际 data/rules.db。输出：

```text
old_token_accepted_after_password_reset=true
old_token_accepted_after_disable=true
fresh_login_blocked_after_disable=true
```

**建议：** 以 userID 索引并撤销会话；改密、禁用、删除时原子推进用户认证代际。ValidateToken 校验代际/状态，兼顾“正在登录”和“正在禁用”并发。明确修改自己密码后当前会话的处理方式。
**验收：** 两个 old_token 指标均为 false；增加旧 token 请求受保护 API 的 401 回归，以及并发登录/禁用竞态测试。

### ENG-A02 · P1 · 背压失败先标记去重，重试被当成成功（已复现）

**证据：** `internal/core/dispatcher/dispatcher.go:218` 在 `:233` 的 Submit 之前调用 CheckAndMark；`deduplicator.go:38` 没有预约/提交状态或失败回滚。Submit 出错后留存去重记录。

队列满时首次返回背压；同 eventID、同边在去重窗口内重试时直接 continue，最后返回 nil，但该事件从未入队。若上游据此确认消息，会造成静默数据丢失。此结论针对 OnPluginEmit 调度路径，未推断其他传输路径具有相同行为。

复现：`go run docs/audit-repros-2026-09-09/dedup_repro.go`。创建容量为 1 的隔离队列，故意不启动消费线程来稳定模拟满队列：

```text
fill_ok=true first_attempt_rejected=true retry_falsely_accepted=true
```

**建议：** 去重采用并发安全的 reserve/commit/rollback；只有成功提交的分支保留标记，失败分支允许重试。不能简单把标记后移而引入并发重复提交。
**验收：** 满队列下重试仍明确失败，释放容量后能投递一次；多分支部分失败只重试失败边；覆盖并发相同 ID。

### ENG-A03 · P1 · 配置写入缺少后端语义校验与启动字段保护（静态确认）

**证据：** `internal/server/manager/handlers/config_handlers.go:61` 仅解析结构体、检查非 nil 后即 SaveConfig。路由 `router.go:240` 有管理员鉴权，但没有字段白名单、跨字段校验或重启前验证。`internal/infra/config/loader.go:81` 在启动加载阶段才检查 NATS 模式/端口。

管理员或客户端误传一个无效 NATS mode、错误连接地址或不完整配置，也可能先得到保存成功，下一次启动才失败；UI 的禁用输入不是 API 约束。

**建议：** 引入统一、可复用的 ValidateConfig；明确 bootstrap 字段与可编辑字段，返回逐字段错误。保存前验证、保留 last-known-good、修订号与回滚记录。
**验收：** 非法 mode/端口/互斥组合返回 400 且数据库不变；重启字段有明确流程；并发编辑检测冲突。

### ENG-A04 · P2 · 整体保存配置会丢弃 JSON 隐藏字段（静态确认）

**证据：** `internal/infra/config/types.go:19,39,58` 的密码等字段和 `security.go` 的 Manager Token 使用 `json:"-"`；GetConfig 会隐藏这些值，UpdateConfig 却用新解码的完整对象覆盖整个 YAML 配置。

对于历史数据库/YAML 中已有秘密字段、尚未全部采用环境变量的部署，GET 后原样 PUT 也无法保留这些字段。环境变量重新覆盖的字段不受此数据丢失路径影响。此处是“更新语义”问题，不是声称当前 GET 泄露密码。

**建议：** API 使用显式可编辑 DTO/PATCH，与持久化对象分离；保留隐藏字段或迁移为 secret reference，清除秘密必须是独立明确操作。
**验收：** 合成秘密写入临时配置后 GET/PUT 不被清空，也不出现在响应或日志中。

### ENG-A05 · P2 · 登录与内存会话缺少容量治理（静态确认）

**证据：** `auth_handlers.go:42`、`userauth/service.go:152,600` 每次成功登录新建会话；sessions map 没有每用户/全局上限。ValidateToken 只在再次访问已过期 token 时清理；没有周期清理。当前 Manager 路由未发现登录限速或 bcrypt 并发限制。

高频登录可消耗 CPU；长期运行且客户端反复登录不注销时，未再次使用的过期会话仍占内存。未对运行系统进行压测。

**建议：** 账号+来源限速、bcrypt 有界并发、会话数量限制及后台到期回收；统一错误响应，避免账号状态泄露。
**验收：** 临时服务限速行为可测，过期会话即便没有再次请求也被回收，长时间会话数量有上界。

### ENG-A06 · P2 · 证书上传未实现专用大小与文件保护（静态确认）

**证据：** `config_handlers.go:99` 使用 FormFile/SaveUploadedFile，目录权限为 0755，没有专用 MaxBytesReader、证书格式校验、不可覆盖命名或密钥权限策略；对照 `handlers/plugin_trust.go:41`，插件上传已有显式限额。接口为管理员专用，不能描述成匿名文件上传漏洞。

**建议：** 在 multipart 解析前限制总大小；服务端生成文件名、拒绝覆盖，校验证书/私钥用途，私钥 0600、目录 0700。无需在报告中断言未验证的目录穿越。
**验收：** 超限返回 413，无遗留文件；无效证书拒绝，私钥权限与同名保护通过。

### ENG-A07 · P1（发布门槛）· 历史凭证仍需处理（历史路径确认）

**证据：** `git log --all -- configs/nats-sun-back.creds configs/nats-sun2.creds` 返回历史提交 `6bc4bc5`。当前删除文件不会删除 Git 历史。仅确认这些路径进过历史，本轮未验证凭证当前是否有效。

**建议：** 在对外推送前完成凭证轮换记录、全历史 secret scan、授权的历史清理或干净发布历史。不要把凭证内容粘贴进审计报告。
**验收：** 扫描报告、轮换记录、最终公开历史检查可复核。

## 已确认的改进与限制

- Dispatcher 入口和每个下游分支 Clone，Observer 接收 EventObservation，已明显降低共享 Metadata 并发崩溃风险；相关 race 测试通过。
- 该防护仍依赖 SDK Clone 的输入类型契约，见 SDK 同日综合报告的 SDK-A02/A03。
- 规则列表已有同步 ref 锁与刷新回归用例，本轮 UI 测试通过；未做浏览器视觉或全部页面交互人工验收。
- 用户密码采用 bcrypt；配置/用户管理 API 的管理员校验存在。没有把旧的未使用 AuthMiddleware 当成当前路由鉴权漏洞。
- 内嵌 NATS 默认与单 artifact 多实例结构有测试支持，不代表已经完成网络分区、宕机恢复和 72 小时压力验证。
- `loader.go:26` 仍先读 YAML；删除配置文件后单文件启动尚未实现。运行数据库存在也不等于可删除 YAML。
- AGPL-3.0-only 已落地；本轮不再把许可证选择列为待定。正式分发的依赖/来源/许可证打包仍需验收。
- 运行环境 Node 20 与英文 README 推荐 Node 22 存在差异，应收敛支持矩阵；本轮测试通过不代表全部 Node 版本受支持。

## 建议顺序与发布条件

1. 先修 ENG-A01/A02，分别阻断撤权失效和事件静默丢失。
2. 完成 ENG-A03/A04 配置 API 与回滚；修复 SDK 日志生命周期、插件 JS 导出边界和 S7 Stop。
3. 再补 ENG-A05/A06，执行 1000 次部署/取消部署资源稳定性、背压重试、配置损坏恢复。
4. 在隔离设备环境验证 S7 100ms/500ms/20s、MQTT 断网重连、规则停止与写入超时行为。
5. 处理 ENG-A07，使用干净发布提交重新跑全套测试及签名构建。

本轮只生成审计报告与复现资料；“建议”不代表已修复。以前报告的“通过/有条件通过”不覆盖本轮新增缺陷。
