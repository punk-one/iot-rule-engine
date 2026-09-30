# iot-rule-plugin-sdk v1 引擎接入约定

引擎依赖 `github.com/punk-one/iot-rule-plugin-sdk v1.0.0`。同级本地开发由父目录
`go.work` 指向 SDK 工作树；正式构建必须使用已发布 tag 并记录 module 校验值。

## 固定握手

所有普通插件和 Connector 加载入口必须调用 `sdk.DefaultHandshakeConfig()`。
`sdk.HandshakeConfig` 仅为旧源码兼容保留，可能被第三方代码修改，不能作为引擎
加载配置。

## 单插件进程池

普通插件必须由 `sdk.ServeFactory` 构建并暴露 `plugin_factory` dispense key。Loader
按解析软链后的 artifact 绝对路径缓存唯一 go-plugin Client；每个规则节点再次
Dispense，获得独立 Plugin/RPC 对象。旧 `sdk.Serve` 插件会被共享进程池拒绝，避免
不同规则误共享一个可变实现对象。

每次 Start 使用 `MuxBroker.NextId()` 生成唯一 `StartArgs.BrokerID`，确保同一进程中
并发启动多个规则时，Health、State、Emitter 和 Connector 反向 RPC 不会串线。

活动规则实例存在时禁止卸载、替换或重载 artifact。停止规则只调用对应实例的 Stop
并减少引用计数，不杀死共享进程；Loader 卸载或引擎关闭时才结束进程。

Connector Host 固定每个 Connector 插件一个进程。`PreferredProviderCount` 与
`MaxResourcesPerProvider` 不再用于扩展 OS 进程，资源隔离和并发限制在同一 Connector
进程内完成。

## RPC 超时

SDK 在 RPC deadline 到期时关闭整个失效的 `net/rpc` client，以释放无法单独取消的
挂起调用。引擎把该错误视为插件连接失效，后续应销毁当前实例并通过既有生命周期
重新加载，不能继续复用 client。

## Event 所有权

Dispatcher、Scheduler、Observer 和插件执行边界遵循“每个调度任务独占 Event”。
引擎不能依赖插件调用返回后继续保留或修改传入 map，也不能把调度字段放回 Metadata。

## 发布验证

每次 SDK v1 更新至少运行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

公开 release 还必须使用冻结的 v1.0.0 插件二进制执行握手、Info、Start、事件、
Health、State、Connector 和 Stop 兼容测试。
