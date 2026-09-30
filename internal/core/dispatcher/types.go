package dispatcher

import (
	"sync"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
)

// NodeID 节点ID类型
type NodeID string

// RuleID 规则ID类型
type RuleID string

// RuleRuntime 规则运行时状态
// 管理单个规则的所有插件和边关系
type RuleRuntime struct {
	RuleID  string
	Plugins map[string]client.PluginClient // nodeID -> client.PluginClient
	Edges   map[string][]string     // fromNodeID -> []toNodeID
	mu      sync.RWMutex            // 保护并发访问
}
