package dispatcher

import (
	"github.com/punk-one/iot-rule-engine/internal/core/plugin/rpc"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// PluginRuntime 表示插件的运行时信息
type PluginRuntime struct {
	NodeID string
	Type   sdk.PluginType

	// RPC 客户端（用于向插件发送事件）
	// 当前临时使用 EngineRPCServer，后续可替换为 PluginRPC
	RPC *rpc.EngineRPCServer
}
