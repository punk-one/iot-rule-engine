package client

import (
	"fmt"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// ExternalPluginClient 外部插件客户端（通过 RPC 通信）
type ExternalPluginClient struct {
	nodeID string
	plugin sdk.Plugin
	rpc    *sdk.PluginRPCClient // 使用 PluginRPCClient
}

// NewExternalPluginClient 创建外部插件客户端
func NewExternalPluginClient(nodeID string, plugin sdk.Plugin, rpc *sdk.PluginRPCClient) *ExternalPluginClient {
	return &ExternalPluginClient{
		nodeID: nodeID,
		plugin: plugin,
		rpc:    rpc,
	}
}

// Info 获取插件信息
func (c *ExternalPluginClient) Info() sdk.PluginInfo {
	return c.plugin.Info()
}

// Start 启动插件
func (c *ExternalPluginClient) Start() error {
	// 外部插件已经在 Engine 中启动，这里不需要再次启动
	return nil
}

// Stop 停止插件
func (c *ExternalPluginClient) Stop() error {
	return c.plugin.Stop()
}

// SendEvent 通过 RPC 发送事件到外部插件
func (c *ExternalPluginClient) SendEvent(e sdk.Event) error {
	if c.rpc == nil {
		return fmt.Errorf("RPC client is nil for node %s", c.nodeID)
	}
	// 使用 PluginRPC.ReceiveEvent 发送事件
	return c.rpc.ReceiveEvent(e)
}

// SendEvents 通过 RPC 发送批量事件
func (c *ExternalPluginClient) SendEvents(events []sdk.Event) error {
	if c.rpc == nil {
		return fmt.Errorf("RPC client is nil for node %s", c.nodeID)
	}
	return c.rpc.ReceiveEvents(events)
}
