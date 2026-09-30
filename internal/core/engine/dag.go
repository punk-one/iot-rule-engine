package engine

import (
	"encoding/json"
	"fmt"

	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// DAG 表示规则的有向无环图
type DAG struct {
	RuleID   string
	RuleFlow *schema.RuleFlow
	Plugins  map[string]sdk.Plugin       // 节点ID -> Plugin 实例
	Configs  map[string]sdk.PluginConfig // 节点ID -> Plugin 配置
	loader   *plugin.Loader              // 插件加载器（可选）
}

// BuildDAG 从 RuleFlow 构建 DAG 图
func BuildDAG(ruleFlow *schema.RuleFlow) (*DAG, error) {
	return BuildDAGWithLoader(ruleFlow, nil)
}

// BuildDAGWithLoader 从 RuleFlow 构建 DAG 图
func BuildDAGWithLoader(ruleFlow *schema.RuleFlow, loader *plugin.Loader) (*DAG, error) {
	if ruleFlow == nil {
		return nil, fmt.Errorf("ruleFlow cannot be nil")
	}

	// 验证 DAG 是否有循环依赖
	if err := ValidateDAG(ruleFlow); err != nil {
		return nil, fmt.Errorf("DAG validation failed: %w", err)
	}

	dag := &DAG{
		RuleID:   ruleFlow.ID,
		RuleFlow: ruleFlow,
		Plugins:  make(map[string]sdk.Plugin),
		Configs:  make(map[string]sdk.PluginConfig),
		loader:   loader,
	}

	// 构建插件映射（先创建所有插件实例）
	for _, nodeConfig := range ruleFlow.Nodes {
		plugin, pluginConfig, err := createPlugin(nodeConfig, loader)
		if err != nil {
			return nil, fmt.Errorf("failed to create plugin for node %s: %w", nodeConfig.ID, err)
		}

		// 初始化插件
		if err := plugin.Init(pluginConfig); err != nil {
			return nil, fmt.Errorf("failed to init plugin for node %s: %w", nodeConfig.ID, err)
		}

		dag.Plugins[nodeConfig.ID] = plugin
		dag.Configs[nodeConfig.ID] = pluginConfig
	}

	pkglog.Info("Built DAG for rule %s with %d plugins", ruleFlow.ID, len(dag.Plugins))
	return dag, nil
}

// createPlugin 根据节点配置创建对应的 Plugin 实例
func createPlugin(nodeConfig *schema.NodeConfig, loader *plugin.Loader) (sdk.Plugin, sdk.PluginConfig, error) {
	// 创建 PluginConfig（序列化为 JSON 以支持 RPC 传递）
	cfgBytes, err := json.Marshal(nodeConfig.Config)
	if err != nil {
		return nil, sdk.PluginConfig{}, fmt.Errorf("failed to serialize config: %w", err)
	}
	pluginConfig := sdk.PluginConfig{
		Raw: cfgBytes,
	}

	// 确定插件 ID
	pluginID := nodeConfig.Type
	if nodeConfig.Plugin != "" {
		pluginID = nodeConfig.Plugin
	}

	// 从插件加载器加载插件（包括原来的内置插件，现在都作为外部插件）
	if loader != nil {
		pluginClient, err := loader.GetPluginClient(pluginID)
		if err == nil && pluginClient != nil {
			// 使用外部插件的 RPC 客户端
			pkglog.Debug("Using plugin: %s for node %s", pluginID, nodeConfig.ID)
			return pluginClient.RPCClient, pluginConfig, nil
		}
	}

	// 如果找不到，返回错误
	return nil, pluginConfig, fmt.Errorf("plugin not found: %s", pluginID)
}

// GetPlugin 根据节点ID获取插件
func (d *DAG) GetPlugin(nodeID string) (sdk.Plugin, bool) {
	plugin, exists := d.Plugins[nodeID]
	return plugin, exists
}

// GetPluginConfig 获取节点的插件配置
func (d *DAG) GetPluginConfig(nodeID string) (sdk.PluginConfig, bool) {
	config, exists := d.Configs[nodeID]
	return config, exists
}

// GetNodeConfig 获取节点的原始配置（用于获取 Wires 等信息）
func (d *DAG) GetNodeConfig(nodeID string) *schema.NodeConfig {
	for _, nodeConfig := range d.RuleFlow.Nodes {
		if nodeConfig.ID == nodeID {
			return nodeConfig
		}
	}
	return nil
}

// GetNodeType 获取节点类型
func (d *DAG) GetNodeType(nodeID string) string {
	nodeConfig := d.GetNodeConfig(nodeID)
	if nodeConfig != nil {
		return nodeConfig.Type
	}
	return "unknown"
}
