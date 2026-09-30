package engine

import (
	"fmt"
	"strings"

	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// ValidateDAG 验证 DAG 是否符合规范
// 检查项：
// 1. 循环依赖
// 2. Source 节点入度 = 0
// 3. Sink 节点出度 = 0
// 4. Processor 节点入度 ≥1 且 出度 ≥1
// 5. 不允许孤立节点（必须连线）
func ValidateDAG(ruleFlow *schema.RuleFlow) error {
	if ruleFlow == nil || len(ruleFlow.Nodes) == 0 {
		return nil
	}

	// 构建邻接表（同时包含 wires 和 routes）
	adjList := make(map[string][]string)
	nodeSet := make(map[string]bool)

	for _, node := range ruleFlow.Nodes {
		nodeSet[node.ID] = true
		neighbors := make([]string, 0, len(node.Wires)+len(node.Routes))
		for _, edge := range collectNodeTopologyEdges(node) {
			neighbors = append(neighbors, edge.To)
		}
		adjList[node.ID] = neighbors
	}

	// 1. 检查循环依赖
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycle func(nodeID string) bool
	hasCycle = func(nodeID string) bool {
		visited[nodeID] = true
		recStack[nodeID] = true

		for _, nextID := range adjList[nodeID] {
			if !nodeSet[nextID] {
				// 引用了不存在的节点
				pkglog.Error("Node %s references non-existent node %s", nodeID, nextID)
				continue
			}

			if !visited[nextID] {
				if hasCycle(nextID) {
					return true
				}
			} else if recStack[nextID] {
				// 发现循环
				return true
			}
		}

		recStack[nodeID] = false
		return false
	}

	// 检查所有节点是否有循环
	for nodeID := range nodeSet {
		if !visited[nodeID] {
			if hasCycle(nodeID) {
				return fmt.Errorf("cycle detected in DAG for rule %s", ruleFlow.ID)
			}
		}
	}

	// 2. 计算每个节点的入度和出度
	inDegree := make(map[string]int)
	outDegree := make(map[string]int)

	// 初始化
	for _, node := range ruleFlow.Nodes {
		inDegree[node.ID] = 0
		outDegree[node.ID] = len(collectNodeTopologyEdges(node))
	}

	// 计算入度
	for _, edge := range collectTopologyEdges(ruleFlow) {
		if nodeSet[edge.To] {
			inDegree[edge.To]++
		}
	}

	// 3. 检查节点类型和入度出度约束
	// 需要获取插件信息来确定节点类型
	// 注意：这里不实际加载插件，只用于获取插件信息
	loader := plugin.NewLoader("") // 用于获取插件信息，但不实际加载

	for _, nodeConfig := range ruleFlow.Nodes {
		nodeID := nodeConfig.ID
		inDeg := inDegree[nodeID]
		outDeg := outDegree[nodeID]

		// 获取插件类型
		pluginType, err := getNodePluginType(nodeConfig, loader)
		if err != nil {
			// 如果无法确定插件类型，跳过类型检查（但会记录警告）
			pkglog.Warn("Cannot determine plugin type for node %s: %v", nodeID, err)
			continue
		}

		// 检查 Source 节点：入度必须 = 0
		if pluginType == sdk.PluginTypeSource {
			if inDeg != 0 {
				pkglog.Warn("Source node %s should have in-degree = 0, but got %d", nodeID, inDeg)
			}
		}

		// 检查 Sink 节点：出度必须 = 0
		if pluginType == sdk.PluginTypeSink {
			if outDeg != 0 {
				pkglog.Warn("Sink node %s should have out-degree = 0, but got %d", nodeID, outDeg)
			}
		}

		// 检查 Processor 节点：入度 ≥1 且 出度 ≥1
		if pluginType == sdk.PluginTypeProcessor {
			if inDeg < 1 {
				pkglog.Warn("Processor node %s has in-degree < 1", nodeID)
			}
			if outDeg < 1 {
				pkglog.Warn("Processor node %s has out-degree < 1", nodeID)
			}
		}

		// 4. 检查孤立节点：不允许没有连线的节点
		if inDeg == 0 && outDeg == 0 {
			pkglog.Warn("Isolated node %s found: node should have at least one connection", nodeID)
		}
	}

	// 5. 检查与 Validate Routes
	for _, nodeConfig := range ruleFlow.Nodes {
		for _, route := range nodeConfig.Routes {
			if !nodeSet[route.To] {
				pkglog.Warn("Route from node %s references non-existent node %s (label: %s)", nodeConfig.ID, route.To, route.Label)
			}
		}
	}

	pkglog.Debug("DAG validation passed for rule %s", ruleFlow.ID)
	return nil
}

// getNodePluginType 获取节点的插件类型
func getNodePluginType(nodeConfig *schema.NodeConfig, loader *plugin.Loader) (sdk.PluginType, error) {
	// 确定插件 ID
	pluginID := nodeConfig.Type
	if nodeConfig.Plugin != "" {
		pluginID = nodeConfig.Plugin
	}

	// 检查插件（通过 loader 获取信息）
	if loader != nil {
		for _, candidate := range pluginIdentifierVariants(pluginID) {
			pluginClient, err := loader.GetPluginClient(candidate)
			if err == nil && pluginClient != nil {
				return pluginClient.Info.Type, nil
			}
			if path, err := loader.ResolvePluginPath(candidate); err == nil {
				if info, err := loader.VerifyPlugin(path); err == nil && info != nil {
					return info.Type, nil
				}
			}
		}

		for _, pluginClient := range loader.ListPluginClients() {
			if pluginIdentifierMatches(pluginClient.Info.ID, pluginID) ||
				pluginIdentifierMatches(pluginClient.Name, pluginID) ||
				pluginIdentifierMatches(pluginClient.Info.Name, pluginID) {
				return pluginClient.Info.Type, nil
			}
		}
	}

	return "", fmt.Errorf("plugin not found: %s", pluginID)
}

func pluginIdentifierVariants(identifier string) []string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return nil
	}

	candidates := []string{trimmed}
	for _, pair := range [][2]string{{"-", "_"}, {"_", "-"}} {
		variant := strings.ReplaceAll(trimmed, pair[0], pair[1])
		if variant != trimmed {
			candidates = append(candidates, variant)
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}
	return unique
}

func pluginIdentifierMatches(actual, requested string) bool {
	for _, candidate := range pluginIdentifierVariants(requested) {
		if actual == candidate {
			return true
		}
	}
	return false
}
