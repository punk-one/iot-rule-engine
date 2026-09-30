package engine

import (
	"fmt"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin/instance"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// BuildPluginInstances 将规则配置转换为一系列插件运行实例
func BuildPluginInstances(rule *schema.RuleFlow) []*instance.PluginInstance {
	var result []*instance.PluginInstance

	for _, node := range rule.Nodes {
		// 确定插件 ID（与 createPlugin 逻辑一致）
		pluginID := node.Type
		if node.Plugin != "" {
			pluginID = node.Plugin
		}

		// 如果插件 ID 为空，跳过
		if pluginID == "" {
			continue
		}

		// ID 格式: plugin#rule#node
		// 确保全局唯一
		id := fmt.Sprintf(
			"%s#%s#%s",
			pluginID,
			rule.ID,
			node.ID,
		)

		result = append(result, &instance.PluginInstance{
			ID:     id,
			Plugin: pluginID, // 使用实际的插件名称
			RuleID: rule.ID,
			NodeID: node.ID,
			Config: cloneConfigMap(node.Config),
		})
	}
	return result
}
