# UI与运维规范

本文档由以下文件整合而成：
- 规则引擎_ui_标准规范_v_1.md
- UI配置优化说明.md

---

## 来源：规则引擎_ui_标准规范_v_1.md

# 规则引擎 UI 标准规范（V1.1）

> 本规范定义规则引擎在 **运行态（Runtime）** 下的 UI 标准，
> 覆盖 **规则总览、拓扑运行视图、节点详情、通道监控** 四个核心页面，
> 并给出对应的 **REST API / WebSocket 协议 / 前端实现建议**。

---

## 1. 设计目标与原则

### 1.1 设计目标

- 让规则引擎 **可观测、可排障、可运维**
- 5 分钟判断规则健康度
- 10 分钟定位问题节点或通道

### 1.2 设计原则

1. **运行态 ≠ 编辑态**（Canvas 仅作为拓扑复用）
2. **实例优先**（RuleInstance / NodeInstance / EdgeInstance）
3. **图 + 指标 + Trace 联动**
4. **前端无业务逻辑，仅展示与联动**

---

## 2. 页面一：规则运行总览（Rule Overview）

### 2.1 页面定位

- 运维入口首页
- 聚焦规则级别健康状态

---

### 2.2 页面结构

```
┌────────────────────────────────────────┐
│ 规则运行总览                            │
├────────────────────────────────────────┤
│ 状态卡片区（KPI）                      │
├────────────────────────────────────────┤
│ RuleInstance 列表                      │
└────────────────────────────────────────┘
```

---

### 2.3 核心指标（KPI）

| 指标 | 说明 |
|----|----|
| Running Rules | status=running |
| Failed Rules | status=failed |
| Total Throughput | events_in / min |
| Last Event Time | max(last_event_at) |

---

### 2.4 RuleInstance 列表字段

| 字段 | 来源 |
|----|----|
| Rule Name | RuleDefinition |
| Version | RuleInstance.Version |
| Status | RuleInstance.Status |
| Nodes | len(nodes) |
| Events In | metrics.events_in |
| Events Out | metrics.events_out |
| Errors | metrics.events_failed |
| Action | 查看运行态 |

---

### 2.5 REST API

```http
GET /api/v1/runtime/rules
```

```json
[
  {
    "rule_id": "r1",
    "name": "device_alarm",
    "version": "v3",
    "status": "running",
    "metrics": {
      "events_in": 10234,
      "events_out": 10012,
      "events_failed": 3
    }
  }
]
```

---

## 3. 页面二：规则拓扑运行视图（Runtime DAG）

### 3.1 页面定位

- 规则运行态主视图
- 可视化 DAG + 实时状态

---

### 3.2 页面结构

```
┌────────────────────────────────────────┐
│ Rule Info Bar                          │
├────────────────────────────────────────┤
│ Runtime DAG（React Flow）              │
└────────────────────────────────────────┘
```

---

### 3.3 节点状态映射

| NodeStatus | UI 表现 |
|----|----|
| running | 绿色光圈 |
| idle | 灰色 |
| error | 红色闪烁 |
| stopped | 深灰 |

---

### 3.4 边（Edge）状态

| 条件 | 表现 |
|----|----|
| 正常 | 实线 + 动效 |
| Lag 增长 | 虚线 |
| Dropped > 0 | 红色标记 |

---

### 3.5 REST API

```http
GET /api/v1/runtime/rules/{ruleId}
```

返回完整 RuleInstance（含 nodes / edges）

---

### 3.6 WebSocket（状态 & Trace）

```text
ws://host/api/v1/runtime/ws?rule_id={ruleId}
```

消息类型：
- node_status
- edge_metrics
- trace_event

---

## 4. 页面三：节点运行详情（Node Inspector）

### 4.1 页面定位

- 节点级排障主页面

---

### 4.2 页面结构

```
┌────────────────────────────────────────┐
│ Node Header                            │
├────────────────────────────────────────┤
│ Tabs: Info | Metrics | Errors | Trace  │
└────────────────────────────────────────┘
```

---

### 4.3 基本信息（Info）

| 字段 | 来源 |
|----|----|
| PluginID | NodeInstance.PluginID |
| Version | NodeInstance.Version |
| Status | NodeInstance.Status |
| StartedAt | NodeInstance.StartedAt |

---

### 4.4 实时指标（Metrics）

- EventsIn / EventsOut（折线）
- AvgLatencyMs（折线）

---

### 4.5 最近错误（Errors）

```json
{
  "time": "2025-12-16T10:23:01Z",
  "message": "HTTP 500",
  "event_id": "evt-9a12"
}
```

---

### 4.6 REST API

```http
GET /api/v1/runtime/rules/{ruleId}/nodes/{nodeId}
```

---

## 5. 页面四：通道 / 边监控（Edge Monitor）

### 5.1 页面定位

- 数据流排障页面

---

### 5.2 页面结构

```
┌────────────────────────────────────────┐
│ Edge Header                            │
├────────────────────────────────────────┤
│ Tabs: Metrics | Flow | Diagnosis       │
└────────────────────────────────────────┘
```

---

### 5.3 EdgeMetrics

| 字段 | 含义 |
|----|----|
| Published | 上游发布 |
| Consumed | 下游消费 |
| Dropped | 丢弃 |
| Lag | Published - Consumed |

---

### 5.4 自动诊断规则

- Lag 持续增长 → 下游消费慢
- Published > 0 & Consumed = 0 → 未订阅
- Dropped > 0 → Sink 异常

---

### 5.5 REST API

```http
GET /api/v1/runtime/rules/{ruleId}/edges/{edgeId}
```

---

## 6. 页面联动规范

| 操作 | 跳转 |
|----|----|
| Overview → Rule | Runtime DAG |
| DAG Node Click | Node Inspector |
| DAG Edge Click | Edge Monitor |
| Trace Event | DAG 高亮 |

---

## 7. 前端技术实现建议

- DAG：React Flow（只读模式）
- 实时数据：WebSocket + Zustand
- 图表：ECharts / Recharts
- Trace：虚拟列表

---

## 8. 总结

> UI 标准规范 V1.1 的目标不是“好看”，而是：
> **让规则引擎成为可观测系统，而不是黑盒。**

---

**版本：UI Standard v1.1**



---

## 来源：UI配置优化说明.md

# UI 配置优化说明

## 问题描述

之前的配置 Schema 使用了 `additionalProperties: true`，导致 rjsf 无法正确渲染对象字段，显示的是动态添加字段的界面（"newKey"、"newKey-1" 等占位符），用户体验很差。

## 解决方案

将以下字段改为 **JSON 字符串格式**，使用 **Monaco Editor** 编辑：

1. **Template Transform** 的 `template` 字段
2. **TimescaleDB Sink** 的 `columns` 字段

## 修改内容

### 1. Template Transform Plugin

**之前**:
```json
{
  "template": {
    "device_id": "{{payload.deviceId}}",
    "value": "{{value}}"
  }
}
```

**现在**:
```json
{
  "template": "{\"device_id\": \"{{payload.deviceId}}\", \"value\": \"{{value}}\"}"
}
```

**UI 体验**:
- 点击 `template` 字段会打开 Monaco Editor
- 支持 JSON 语法高亮和格式化
- 可以直接编辑 JSON 对象

### 2. TimescaleDB Sink Plugin

**之前**:
```json
{
  "columns": {
    "device_id": "payload.device_id",
    "value": "payload.value"
  }
}
```

**现在**:
```json
{
  "columns": "{\"device_id\": \"payload.device_id\", \"value\": \"payload.value\"}"
}
```

**UI 体验**:
- 点击 `columns` 字段会打开 Monaco Editor
- 支持 JSON 语法高亮和格式化
- 可以直接编辑键值对映射

## 插件兼容性

插件代码已更新，支持两种格式：
1. **JSON 字符串格式**（新格式，推荐用于 UI）
2. **对象格式**（旧格式，向后兼容）

插件会自动检测并解析：
- 如果是字符串，解析为 JSON
- 如果已经是对象，直接使用

## 使用示例

### Template Transform 配置

在 UI 中：
1. 点击 `template` 字段
2. Monaco Editor 会打开
3. 输入以下 JSON（格式化后）：
```json
{
  "device_id": "{{payload.deviceId}}",
  "metric": "{{key}}",
  "value": "{{value}}",
  "ts": "{{payload.ts}}"
}
```

### TimescaleDB Sink 配置

在 UI 中：
1. 点击 `columns` 字段
2. Monaco Editor 会打开
3. 输入以下 JSON（格式化后）：
```json
{
  "device_id": "payload.device_id",
  "metric": "payload.metric",
  "avg_1m": "payload.avg_1m",
  "max_1m": "payload.max_1m",
  "time": "ts"
}
```

## 优势

1. ✅ **更好的用户体验**：Monaco Editor 提供语法高亮、自动补全、格式化
2. ✅ **更清晰的配置**：JSON 格式更直观，易于理解和编辑
3. ✅ **向后兼容**：插件仍支持对象格式，不影响现有配置
4. ✅ **统一格式**：与 CEL 表达式等其他代码字段保持一致

## 注意事项

1. JSON 字符串中的引号需要转义（`\"`）
2. 在 Monaco Editor 中编辑时，会自动处理转义
3. 保存时会自动验证 JSON 格式
4. 如果 JSON 格式错误，会显示错误提示
