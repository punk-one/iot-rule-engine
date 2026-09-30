package client

import (
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// PluginClient 统一的内外插件接口
// 外部插件通过 RPC 实现，内置插件通过 Loopback 实现
type PluginClient interface {
	// Info 获取插件信息
	Info() sdk.PluginInfo

	// Start 启动插件
	Start() error

	// Stop 停止插件
	Stop() error

	// SendEvent Dispatcher 唯一调用点，发送事件到插件
	SendEvent(e sdk.Event) error

	// SendEvents 批量发送事件
	// 语义：默认非原子，插件遇到错误应尽快返回，调用方按批次失败处理。
	SendEvents(events []sdk.Event) error
}
