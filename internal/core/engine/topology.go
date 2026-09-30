package engine

import (
	"fmt"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

type topologyEdge struct {
	From  string
	To    string
	Label string
}

func collectTopologyEdges(ruleFlow *schema.RuleFlow) []topologyEdge {
	if ruleFlow == nil {
		return nil
	}

	edges := make([]topologyEdge, 0, len(ruleFlow.Nodes))
	for _, node := range ruleFlow.Nodes {
		edges = append(edges, collectNodeTopologyEdges(node)...)
	}
	return edges
}

func collectNodeTopologyEdges(node *schema.NodeConfig) []topologyEdge {
	if node == nil {
		return nil
	}

	edges := make([]topologyEdge, 0, len(node.Wires)+len(node.Routes))
	for _, to := range node.Wires {
		if to == "" {
			continue
		}
		edges = append(edges, topologyEdge{From: node.ID, To: to})
	}
	for _, route := range node.Routes {
		if route.To == "" {
			continue
		}
		edges = append(edges, topologyEdge{From: node.ID, To: route.To, Label: route.Label})
	}
	return edges
}

func collectDispatcherEdges(ruleFlow *schema.RuleFlow) map[string][]string {
	edges := make(map[string][]string)
	if ruleFlow == nil {
		return edges
	}

	for _, node := range ruleFlow.Nodes {
		if len(node.Wires) == 0 {
			continue
		}
		wires := make([]string, 0, len(node.Wires))
		for _, to := range node.Wires {
			if to == "" {
				continue
			}
			wires = append(wires, to)
		}
		if len(wires) > 0 {
			edges[node.ID] = wires
		}
	}

	return edges
}

func collectLogicalRouters(ruleFlow *schema.RuleFlow) map[string]map[string]string {
	routers := make(map[string]map[string]string)
	if ruleFlow == nil {
		return routers
	}

	for _, node := range ruleFlow.Nodes {
		if len(node.Routes) == 0 {
			continue
		}
		logicalEdges := make(map[string]string, len(node.Routes))
		for _, route := range node.Routes {
			if route.Label == "" || route.To == "" {
				continue
			}
			logicalEdges[route.Label] = route.To
		}
		if len(logicalEdges) > 0 {
			routers[node.ID] = logicalEdges
		}
	}

	return routers
}

func topologyEdgeID(edge topologyEdge) string {
	if edge.Label == "" {
		return fmt.Sprintf("%s-%s", edge.From, edge.To)
	}
	return fmt.Sprintf("%s-%s-%s", edge.From, edge.Label, edge.To)
}

func topologyEdgeTopic(ruleID string, edge topologyEdge) string {
	if edge.Label == "" {
		return fmt.Sprintf("rule.edge.%s.%s.%s", ruleID, edge.From, edge.To)
	}
	return fmt.Sprintf("rule.route.%s.%s.%s.%s", ruleID, edge.From, edge.Label, edge.To)
}
