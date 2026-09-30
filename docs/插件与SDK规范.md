# 插件与 SDK 规范（基于当前 `sdk/` 代码）

本文档以仓库内 `sdk/` 目录当前实现为准，替代历史拼接版文档。

适用代码版本（文档更新时）：
- `sdk/interfaces.go`
- `sdk/types.go`
- `sdk/event.go`
- `sdk/defaults.go`
- `sdk/bridge.go`
- `sdk/plugin_rpc.go`
- `sdk/engine_rpc.go`
- `sdk/handshake.go`
- `sdk/README.md`

## 1. 目标与边界

本规范仅覆盖插件 SDK 的公共能力与实际接口约束：
- 插件生命周期接口
- 运行时上下文能力（日志、发射、指标）
- 事件模型
- 能力声明与配置入口
- go-plugin RPC 通信约定

不再包含以下内容：
- 历史版本演进长文拼接
- 业务内置插件清单与业务配置样例
- 与 SDK 解耦的前端 UI 实现细节
- 单一插件专题说明（如 Sparkplug 专题）

## 2. SDK 目录职责

当前 `sdk/` 目录分工如下：

- `interfaces.go`: 核心接口定义（`Plugin`、`RuntimeContext`、`Emitter`、`Logger`、`Metrics`、`StateManager`）
- `types.go`: 基础类型（`PluginInfo`、`PluginCapabilities`、`PluginType`、`PluginConfig`、`LogLevel`）
- `event.go`: `Event` 模型及编码辅助
- `defaults.go`: 默认实现（`DefaultLogger`、`DefaultMetrics`、`EmitterAdapter`、`RuntimeContextImpl`）
- `bridge.go`: 插件进程内 RPC 桥接（`PluginRuntimeContext`、RPC Logger/Emitter）
- `plugin_rpc.go`: 插件侧 RPC server/client 封装
- `engine_rpc.go`: 引擎侧 RPC 接口与 client 封装
- `handshake.go`: `go-plugin` 握手配置
- `schema/*.go`: 历史/辅助结构体定义（不属于 `Plugin` 最小开发必需接口）

## 3. 核心类型规范

### 3.1 插件类型 `PluginType`

```go
type PluginType string

const (
    PluginTypeSource    PluginType = "source"
    PluginTypeProcessor PluginType = "processor"
    PluginTypeSink      PluginType = "sink"
    PluginTypeUtility   PluginType = "utility"
)
```

说明：
- 当前代码中的处理型插件类型值是 `"processor"`，不是 `"process"`。
- 新文档、示例和配置应统一使用上述枚举值。

### 3.2 插件元信息 `PluginInfo`

```go
type PluginInfo struct {
    ID           string             `json:"id"`
    Name         string             `json:"name"`
    Version      string             `json:"version"`
    Type         PluginType         `json:"type"`
    Description  string             `json:"description"`
    Author       string             `json:"author"`
    Category     string             `json:"category"`
    Capabilities PluginCapabilities `json:"capabilities"`
}
```

字段约定：
- `ID`: 全局唯一标识
- `Version`: 建议使用语义化版本
- `Type`: 必须来自 `PluginType` 枚举
- `Capabilities`: 声明端口、批处理、Ack、状态化、配置 Schema 等能力

### 3.3 能力声明 `PluginCapabilities`

```go
type PluginCapabilities struct {
    InputPorts   int    `json:"input_ports"`
    OutputPorts  int    `json:"output_ports"`
    SupportBatch bool   `json:"support_batch"`
    SupportAck   bool   `json:"support_ack"`
    ConfigSchema string `json:"config_schema"`
    Stateful     bool   `json:"stateful"`
}
```

建议：
- 如果实现了 `OnEvents` 的高效批处理逻辑，`SupportBatch` 应设置为 `true`。
- 如果插件需要消息确认流程，声明 `SupportAck=true` 并调用 `Emitter.Ack`。
- `ConfigSchema` 用于配置结构描述，建议使用 JSON Schema 字符串（详见 `sdk/README.md` 约定）。

### 3.4 插件配置 `PluginConfig`

```go
type PluginConfig struct {
    Raw json.RawMessage `json:"raw"`
}
```

说明：
- SDK 仅透传原始配置 JSON。
- 插件在 `Init` 中自行解析 `cfg.Raw` 到具体结构。

## 4. 事件模型规范

### 4.1 `Event` 结构

```go
type Event struct {
    ID        string                 `json:"id"`
    Timestamp time.Time              `json:"timestamp"`
    Payload   map[string]interface{} `json:"payload"`
    Metadata  map[string]string      `json:"metadata"`

    RuleID string `json:"-"`
    Source string `json:"-"`
}
```

关键点：
- 对外序列化字段：`id`、`timestamp`、`payload`、`metadata`
- 内部流转字段：`RuleID`、`Source`（`json:"-"`，不会序列化）

### 4.2 编码与创建工具

- `EncodeEvent(e Event) ([]byte, error)`
- `DecodeEvent(data []byte) (Event, error)`
- `NewEvent(payload, metadata)` 自动生成 `ID` 与时间戳

## 5. 插件接口与生命周期

`Plugin` 接口定义如下：

```go
type Plugin interface {
    Info() PluginInfo
    Init(cfg PluginConfig) error
    Start(ctx RuntimeContext) error
    OnEvent(e Event) error
    OnEvents(events []Event) error
    Stop() error
}
```

生命周期顺序：
1. `Info`: 上报插件元信息和能力声明
2. `Init`: 接收并解析配置
3. `Start`: 注入运行时上下文，准备对外发射/记录
4. `OnEvent` / `OnEvents`: 处理单条或批量事件
5. `Stop`: 释放资源并停止后台任务

实现建议：
- `OnEvents` 不应简单循环调用 `OnEvent` 后逐条发射；若声明 `SupportBatch=true`，应优先使用 `Emitter.EmitBatch`。
- 插件内部若包含共享状态，需自行保证并发安全。

## 6. RuntimeContext 与能力接口

### 6.1 `RuntimeContext`

```go
type RuntimeContext interface {
    RuleID() string
    NodeID() string
    Logger() Logger
    Emitter() Emitter
    Metrics() Metrics
}
```

说明：
- 插件通过 `RuntimeContext` 访问引擎能力。
- `StateManager` 在 SDK 中有接口定义，但当前不在 `RuntimeContext` 接口暴露。

### 6.2 `Logger`

```go
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    LogBatch(level LogLevel, messages []string, fields []map[string]interface{})
}
```

实现特性（`bridge.go`）：
- RPC Logger 采用缓冲策略：达到 100 条或 2 秒 flush 一次。

### 6.3 `Emitter`

```go
type Emitter interface {
    Publish(event Event) error
    Ack(eventID string) error
    EmitTo(label string, event Event) error
    EmitBatch(events []Event) error
}
```

路由语义：
- `EmitTo(label, event)` 会在 `event.Metadata["route_label"]` 写入路由标签并发射。

### 6.4 `Metrics`

```go
type Metrics interface {
    IncCounter(name string, labels map[string]string)
    Observe(name string, value float64, labels map[string]string)
}
```

### 6.5 `StateManager`

```go
type StateManager interface {
    GetState(key string, state interface{}) error
    SetState(key string, state interface{}) error
    DeleteState(key string) error
    GenerateKey(keys []string) string
}
```

说明：
- `StateManager` 是 SDK 的状态能力抽象。
- 插件若要使用，需由宿主侧通过扩展上下文注入，当前标准 `RuntimeContext` 不直接提供该方法。

## 7. RPC 与进程通信规范

### 7.1 go-plugin 握手

`handshake.go` 中固定如下：

```go
var HandshakeConfig = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "PUNK_PLUGIN_MAGIC",
    MagicCookieValue: "punk-rule-engine",
}
```

要求：
- 引擎与插件必须使用一致握手配置。

### 7.2 插件 RPC 包装

- SDK 通过 `PluginRPC` 封装 go-plugin 通信。
- `PluginRPCServer` 在 `Start` 中通过 `MuxBroker` 的连接 ID `3` 建立引擎 RPC 通道。

### 7.3 引擎侧 RPC 能力

`EngineRPC` 接口：

```go
type EngineRPC interface {
    Emit(e Event) error
    EmitWithTargets(e Event, toNodeIDs []string) error
    Log(level LogLevel, msg string, fields map[string]interface{})
    LogBatch(level LogLevel, messages []string, fields []map[string]interface{})
    IncCounter(name string, labels map[string]string)
    Observe(name string, value float64, labels map[string]string)
    Ack(eventID string) error
    EmitBatch(events []Event) error
}
```

要点：
- 批量发射由 `EmitBatch` 提供，插件侧可通过 `Emitter.EmitBatch` 直达。
- 旧接口 `EdgeSubject(...)` 在代码中已标注 `Deprecated`。

## 8. ConfigSchema 约定

SDK 代码将 `ConfigSchema` 定义为字符串字段，不做语法强制。

结合 `sdk/README.md` 的当前约定，建议：
- 使用标准 JSON Schema（建议 Draft 2020-12）
- 不在 Schema 内写 `ui:*` 扩展字段
- UI 展示行为由前端层处理，不写入运行时能力协议

## 9. 最小可运行插件模板

```go
package main

import (
    "encoding/json"

    "github.com/hashicorp/go-plugin"
    "github.com/punk-one/iot-rule-engine/sdk"
)

type MyProcessor struct {
    cfg map[string]interface{}
    ctx sdk.RuntimeContext
}

func (p *MyProcessor) Info() sdk.PluginInfo {
    return sdk.PluginInfo{
        ID:      "my-processor",
        Name:    "My Processor",
        Version: "1.5.0",
        Type:    sdk.PluginTypeProcessor,
        Capabilities: sdk.PluginCapabilities{
            SupportBatch: true,
            ConfigSchema: `{"type":"object"}`,
        },
    }
}

func (p *MyProcessor) Init(cfg sdk.PluginConfig) error {
    return json.Unmarshal(cfg.Raw, &p.cfg)
}

func (p *MyProcessor) Start(ctx sdk.RuntimeContext) error {
    p.ctx = ctx
    return nil
}

func (p *MyProcessor) OnEvent(e sdk.Event) error {
    e.Payload["processed"] = true
    return p.ctx.Emitter().Publish(e)
}

func (p *MyProcessor) OnEvents(events []sdk.Event) error {
    for i := range events {
        events[i].Payload["processed"] = true
    }
    return p.ctx.Emitter().EmitBatch(events)
}

func (p *MyProcessor) Stop() error { return nil }

func main() {
    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: sdk.HandshakeConfig,
        Plugins: map[string]plugin.Plugin{
            "plugin": &sdk.PluginRPC{Impl: &MyProcessor{}},
        },
    })
}
```

## 10. 插件开发检查清单

- `Info()` 返回稳定且完整的元信息（尤其 `ID`、`Type`、`Capabilities`）
- `Init()` 只做配置解析和静态校验
- `Start()` 保存 `RuntimeContext`，避免在 `OnEvent` 里重复初始化
- 实现 `OnEvent` 与 `OnEvents`，且两者语义一致
- 需要高吞吐时使用 `EmitBatch`，并在能力中声明 `SupportBatch=true`
- 路由分发使用 `EmitTo`（依赖 `route_label` 元数据）
- 停止时在 `Stop()` 中关闭 goroutine、ticker、连接等资源

## 11. 结论

当前 SDK 已形成稳定的插件协议核心：
- 统一生命周期接口（含批处理）
- 统一运行时能力注入（Logger / Emitter / Metrics）
- 基于 go-plugin 的进程隔离与 RPC 通道

后续文档增补应以 `sdk/` 代码为准，不再采用多文档拼接方式维护。
