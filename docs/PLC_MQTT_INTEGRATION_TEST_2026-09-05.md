# PLC → MQTT 现场联调记录（2026-09-05）

## 结论

8 台 Siemens S7 PLC 均可建立真实 S7 会话，8 条规则均处于 `running`，采集事件通过 MQTT 发布到 `v1/gateway/cnc/telemetry`。订阅端已在一次验证窗口内收齐 cnc101–cnc108，消息结构和毫秒时间戳符合要求。

本次还验证并修复了插件进程复用：统一 Engine 内的 Manager 与 Executor 现在共享同一个插件 Loader。启动 8 条规则后，`connect-s7`、`source-s7`、`connect-mqtt`、`sink-mqtt` 各只有一个 SO 进程；规则之间使用独立插件实例。

## 测试拓扑

每台设备使用一条规则：

```text
S7 ConnectorResource → source-s7 → sink-mqtt → MQTT ConnectorResource
```

- S7 参数：`rack=0`、`slot=1`、`connect_type=pg`
- 采集周期：1000 ms
- MQTT QoS：1
- MQTT retain：false
- MQTT 编码：仅发布事件 payload
- MQTT 凭据只写入临时 SQLite 联调数据库，没有写入仓库文件或日志

## PLC 端点

| 设备 | 地址 | 结果 | PLC 状态码 |
|---|---|---|---:|
| cnc101 | `192.0.2.10:20101` | healthy | 8 |
| cnc102 | `192.0.2.10:20102` | healthy | 8 |
| cnc103 | `192.0.2.10:20103` | healthy | 8 |
| cnc104 | `192.0.2.10:20104` | healthy | 8 |
| cnc105 | `192.0.2.10:20105` | healthy | 8 |
| cnc106 | `192.0.2.10:20106` | healthy | 8 |
| cnc107 | `192.0.2.10:20107` | healthy | 8 |
| cnc108 | `192.0.2.10:20108` | healthy | 8 |

MQTT `198.51.100.10:1883` 的连接资源状态同样为 `healthy`。以上地址均为文档保留地址，
不对应真实设备或生产 Broker。

## 消息结构

实际收到的消息符合：

```json
{
  "device_code": "cnc101",
  "data": {
    "total_out": -1677459456,
    "emerg_stop_alarm": false,
    "auto_cond_alarm": false,
    "air_press_alarm": false,
    "safety_door_alarm": false
  },
  "time": 1788608614286
}
```

`time` 为 Unix epoch 毫秒整数。

## 当前点位限制

本次缺少 cnc101–cnc108 的正式 PLC 点表，因此为了验证协议、调度和 MQTT 全链路，临时使用旧项目中的示例点位：

| 字段 | S7 地址 | 类型 |
|---|---|---|
| total_out | `MD0` | Int32 |
| emerg_stop_alarm | `MX150.0` | Bool |
| auto_cond_alarm | `MX152.7` | Bool |
| air_press_alarm | `MX150.1` | Bool |
| safety_door_alarm | `MX150.2` | Bool |

这些地址都可以读取，但 `total_out` 的实际数值不符合正常产量语义，不能作为生产点表验收依据。上线前必须由 PLC/设备方确认每个字段的地址、数据类型、字节序、缩放系数和精度。

## 回归结果

- 8 条规则启动和运行成功，运行时失败事件为 0
- MQTT 订阅验证一次收齐 8 个设备编号
- 全部插件通过竞态检测、单元测试、静态分析和构建
- Rule Engine 通过 `go test -race ./...`、`go vet ./...` 和 `git diff --check`

## 联调实例

- Manager API：`http://127.0.0.1:11011`
- 临时配置与数据：`/tmp/iot-rule-engine-integration`
- 该实例用于联调，不替代生产部署配置
