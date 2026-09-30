package router

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// Router handles runtime routing based on Logical Edges
// Explicit routing table
// Gray Release support
type Router struct {
	mu      sync.RWMutex
	routers map[string]*RouterNode // nodeID -> RouterNode

	// Gray Release configurations
	grayConfigs map[string]*schema.GrayReleaseConfig // ruleID -> GrayReleaseConfig
}

// NewRouter creates a new Router
func NewRouter() *Router {
	return &Router{
		routers:     make(map[string]*RouterNode),
		grayConfigs: make(map[string]*schema.GrayReleaseConfig),
	}
}

// RegisterRouter registers routing rules for a node
func (r *Router) RegisterRouter(nodeID string, edges map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routers[nodeID] = &RouterNode{
		NodeID:       nodeID,
		LogicalEdges: edges,
	}
}

// UnregisterRouter removes routing rules for a node
func (r *Router) UnregisterRouter(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routers, nodeID)
}

// Resolve identifies target nodes for an event
// Routing logic:
// 1. If event is from Router (IsFromRouter=true), use RouteLabel to find target.
// 2. If event is physical (IsFromRouter=false), fallback or return nil.
//
// In strict mode:
// - Plugins only EmitTo(label)
// - Dispatcher calls Resolve(event) -> []string
func (r *Router) Resolve(e sdk.Event) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Strict check: we only support explicit routing in Router
	// However, if the event comes from "OnPluginEmit(legacy)", IsFromRouter might be false.
	// But in the new architecture, we want to move towards Label based routing.

	// Helper: check if event has routing info
	// sdk.Event might not have IsFromRouter/RouteLabel fields yet!
	// We need to check sdk.Event definition.

	// Assuming sdk.Event has Metadata. We can store routing info in Metadata for now if fields are missing.
	// Or we assume the Caller (Dispatcher) passed us the label.
	// But the signature `Resolve(e sdk.Event)` implies the event carries the intent.

	// Let's look at how we implemented OnPluginEmit in Dispatcher.
	// In the legacy OnPluginEmit, we looked up Edges[fromNode] -> []toNode.
	// That was "Implicit/Physical" routing (no label, just default output).

	// Router adds "Logical/Label" routing.

	// If e.Metadata["route_label"] is set, we use it.
	label, hasLabel := e.Metadata["route_label"]
	fromNode := e.Source

	if hasLabel && fromNode != "" {
		router, ok := r.routers[fromNode]
		if !ok {
			return nil
		}
		if toNode, ok := router.LogicalEdges[label]; ok {
			return []string{toNode}
		}
	}

	return nil
}

// Gray Release methods

// RegisterGrayConfig 注册灰度发布配置
func (r *Router) RegisterGrayConfig(config *schema.GrayReleaseConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grayConfigs[config.RuleID] = config
}

// UnregisterGrayConfig 移除灰度发布配置
func (r *Router) UnregisterGrayConfig(ruleID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.grayConfigs, ruleID)
}

// ResolveWithGrayRelease 解析路由（支持灰度发布）
// 如果规则启用了灰度发布，根据策略决定是否路由到灰度节点
func (r *Router) ResolveWithGrayRelease(ruleID string, e sdk.Event) []string {
	// Gray tagging must never mutate a caller-owned Metadata map.
	e = e.Clone()

	r.mu.RLock()
	defer r.mu.RUnlock()

	// 检查是否有灰度配置
	grayConfig, hasGray := r.grayConfigs[ruleID]
	if !hasGray || !grayConfig.Enabled {
		// 没有灰度配置或未启用，使用普通路由
		return r.Resolve(e)
	}

	// 检查是否应该路由到灰度节点
	shouldRouteToGray := false

	switch grayConfig.Strategy {
	case "percentage":
		// 基于比例的灰度：随机决定
		percentage := grayConfig.Percentage
		if percentage < 0 {
			percentage = 0
		}
		if percentage > 100 {
			percentage = 100
		}
		randValue := rand.Intn(100)
		shouldRouteToGray = randValue < percentage

	case "condition":
		// 基于条件的灰度：检查事件是否匹配条件
		shouldRouteToGray = true
		for key, value := range grayConfig.Condition {
			if e.Metadata[key] != value {
				shouldRouteToGray = false
				break
			}
		}

	default:
		// 未知策略，使用普通路由
		return r.Resolve(e)
	}

	// 如果应该路由到灰度节点，返回灰度目标节点
	if shouldRouteToGray && len(grayConfig.TargetNodes) > 0 {
		// 在事件元数据中添加版本标签（流量染色）
		if e.Metadata == nil {
			e.Metadata = make(map[string]string)
		}
		e.Metadata["gray_version"] = fmt.Sprintf("%d", grayConfig.Version)
		e.Metadata["gray_release"] = "true"

		return grayConfig.TargetNodes
	}

	// 否则使用普通路由
	return r.Resolve(e)
}
