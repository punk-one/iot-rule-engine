package rulesanitize

import "github.com/punk-one/iot-rule-plugin-sdk/schema"

const VirtualSourceNodeID = "rule_source_node"

// CleanupVirtualSourceNodes removes UI-only source nodes that should never be
// persisted to the backend or sent to the executor.
func CleanupVirtualSourceNodes(rule *schema.RuleFlow) {
	if rule == nil || len(rule.Nodes) == 0 {
		return
	}

	cleanNodes := make([]*schema.NodeConfig, 0, len(rule.Nodes))
	for _, node := range rule.Nodes {
		if node == nil {
			continue
		}
		if node.ID == VirtualSourceNodeID {
			continue
		}
		cleanNodes = append(cleanNodes, node)
	}
	rule.Nodes = cleanNodes
}
