# Connector 生命周期设计（v1）

## 三层生命周期

```text
Package Sidecar      长期运行，一个插件产物一个进程
Connector Resource  长期保存配置、门禁、需求与统计
S7 Device Session   首次真实需求建立，只在连续高频需求期间复用
```

规则数量不会增加同一 Connector 插件的进程数。`CreateResource` 不连接 PLC；物理
会话由真实读写触发。MQTT 仍使用持久连接，数据库仍使用连接池。

## v1 策略与兼容

SDK v1 在既有字符串字段上新增 `scheduled_reuse` 常量，没有修改公开结构体、接口或
RPC DTO：

- `scheduled_reuse`：S7 默认，按下一次需求感知复用。
- `per_operation`：严格一次操作一次会话，也保留给 FANUC。
- `persistent`：MQTT 长连接。
- `pooled`：SQL、TDengine、Redis 连接池。
- `stateless`、`idle_timeout`：其他协议使用。

S7 旧值 `shared_duplex`、`split_rw` 在运行时映射到 `scheduled_reuse`，数据库记录不
自动改写；用户保存时 UI 写入新值。显式 `per_operation` 保持严格短连接语义。

## S7 默认调度

默认 `sessionReuseHorizonMillis=2000`、`closeWhenNoDemand=true`：

| 周期 | 默认行为 |
| ---: | --- |
| 100ms / 500ms / 1s | 首次建立，连续采集期间复用 |
| 5s / 20s | 每批建立并关闭 |
| 临时读写 | 完成后关闭，除非同 Resource 已有近期计划采集 |

一次批量读使用一个设备会话，全部点位和 PDU 分段完成后才允许写入。ResourceBus 与
Connector 双层串行同一 Resource；过期 Source Tick 被跳过而不排队补跑。不同资源可
并行，S7 sidecar 同时持有的设备会话上限为 8。

Source 请求使用原有 Payload 扩展字段 `next_demand_in_ms` 和原有 `ResourceToken`
报告需求，不新增 SDK DTO。Source Stop 发送 `release_demand`；Resource Stop/Delete、
配置替换、会话损坏、没有近期需求以及最大年龄边界都会关闭会话。

## 熔断与健康

高频读取不在当前 Tick 立即重试。默认连续 3 次失败打开熔断，初始 1 秒指数退避、
最大 30 秒；期间 Tick 可继续到达但不会产生 TCP 拨号。退避截止后的第一个串行请求
执行 half-open，成功清零，失败继续退避。写入不透明重试。

`scheduled_reuse` 与 `per_operation` 都属于 demand-managed Resource。引擎不对它们
运行周期 Probe，避免探测制造连接需求；初始状态为 `healthy/DEMAND_MANAGED`。真实
操作成功变为 `healthy/OPERATION_OK`，一次或两次连续失败为 `degraded`，第三次起为
`disconnected/OPERATION_FAILED`。UI 对健康状态显示 `Ready`，断开显示
`Unreachable`，不根据 TCP 是否正在 ESTABLISHED 判断业务健康。

## 可观测性与验收

Source 上报实际间隔、扫描耗时和跳过周期；S7 metadata 返回 Session 建立、复用、
关闭、连续失败及熔断状态；每次批读响应包含点位分段数和耗时。

必须验证 100ms、500ms、20s 三组频率：批次不重叠、不积压、同一批次不重复连接；
高频正常采集复用 Session，低频每批关闭；停止、失败和删除后释放；PLC 离线时不出现
连接风暴；MQTT 行为不变。100ms 的目标是
`p99(排队 + 全部分段读取 + 解码) < 70~80ms`，最终能力以现场 PLC、PDU 与网络压测为准。

## 当前聚合边界

同一 PLC Resource 的多个周期共享串行门禁、需求集合和物理会话。多个独立 Source
在同一时刻到期时仍以独立 Connector 请求顺序执行，不跨规则合并点位或改变响应
边界。若要实现“500ms Tick 把多个规则点位合成一条 PDU 批读”，需要一个带响应拆分
语义的引擎级 ScanCoordinator；该能力不能通过 SDK v1 的同步 Execute 调用隐式实现。
