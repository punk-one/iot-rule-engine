package engine

import (
	pluginruntime "github.com/punk-one/iot-rule-engine/internal/core/plugin/instance"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func (e *Executor) SetDeliveryConfig(cfg DeliveryConfig) { e.deliveryConfig = &cfg }

// Sources and replaying processors may emit during Start. Their downstream
// clients must be registered first, regardless of the editor's node order.
func downstreamFirstInstances(rule *schema.RuleFlow, instances []*pluginruntime.PluginInstance) []*pluginruntime.PluginInstance {
	byNode := make(map[string]*pluginruntime.PluginInstance, len(instances))
	for _, inst := range instances {
		byNode[inst.NodeID] = inst
	}
	edges := collectDispatcherEdges(rule)
	seen := make(map[string]bool, len(instances))
	result := make([]*pluginruntime.PluginInstance, 0, len(instances))
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, to := range edges[id] {
			visit(to)
		}
		if inst := byNode[id]; inst != nil {
			result = append(result, inst)
		}
	}
	for _, inst := range instances {
		visit(inst.NodeID)
	}
	return result
}
