# cnc18 S7 点表与规则说明（2026-09-06）

## 交付内容

- `docs/cnc18-profile.rule.json`：可导入/创建的完整规则，拓扑为 `source-s7 → detect-change → sink-mqtt`。
- `docs/cnc18-s7-points.xlsx`：19 个点位的 Excel 模板，可在 source-s7 的 points 编辑区直接导入。
- UI 的 points 编辑区支持 JSON 导入、JSON 导出、Excel 导入、Excel 导出。

规则配置使用：

- Profile：`cnc18-profile`
- 描述：`数控机7号机`
- 采集周期：5 秒
- 批量大小：19
- MQTT topic：`v1/gateway/cnc/telemetry`
- onChange 关注：`total_output`、`stop_alarm`、`cnc_alarm1`～`cnc_alarm4`
- MQTT payload：只保留 `device_code`、`data`、`time`

## 部署前需要创建的连接资源

规则不保存口令，依赖两个已有或新建的 ConnectorResource：

1. `s7-cnc18`：插件 `connect-s7`，填写目标 PLC host、port、rack、slot、connect_type。
2. `mqtt-cnc-telemetry`：插件 `connect-mqtt`，填写 broker、client_id、username 和 secret/password。

连接资源 ID 如与现场不同，应同步修改规则中两个 `connector_ref`。

## onChange 行为

`detect-change` 按 `device_code` 保存快照。规则显式配置 `emit_first:true`，所以启动后的第一个完整快照会发送；后续仅当六个 watched fields 任意一个变化时发送。规则配置 `annotate:false`，不会向消息加入 `_change_*` 字段。

## 范围与默认值

当前数值点使用 PLC 基础类型上下界和默认值 0，这是通信安全兜底，不是工艺报警阈值。现场确认后，应把范围改成真实工程范围。例如温度点可配置：

```json
{
  "scale": 0.1,
  "precision": 1,
  "range": {
    "minimum": 0,
    "maximum": 120,
    "policy": "use_default",
    "default_value": 0
  }
}
```

## 字段兼容说明

用户提供的 `unload_line_alrm` 保持原拼写。此次没有静默修正为 `unload_line_alarm`，避免 MQTT 消费方取不到原字段。

## 测试顺序

1. 在 UI 导入 Excel，确认生成的 points JSON 为 19 个点。
2. 校验并保存规则，但先不部署。
3. 检查两个连接资源为 healthy。
4. 部署规则，观察 source、detect-change、sink 三个节点健康状态。
5. 订阅 `v1/gateway/cnc/telemetry`，验证首帧以及 watched fields 变化后的消息。
6. 确认消息中 `time` 为毫秒时间戳，且没有 `_change_*` 字段。
