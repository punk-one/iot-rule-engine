package instance

import (
	"time"
)

// InstanceStatus 定义实例状态
type InstanceStatus string

const (
	InstanceCreated InstanceStatus = "CREATED"
	InstanceRunning InstanceStatus = "RUNNING"
	InstanceStopped InstanceStatus = "STOPPED"
	InstanceFailed  InstanceStatus = "FAILED"
)

// PluginInstance 插件运行实例
// 对应一个规则图中的节点
type PluginInstance struct {
	ID        string         // 唯一实例ID (plugin#rule#node)
	Plugin    string         // 插件标识 (binary name or path)
	RuleID    string         // 所属规则
	NodeID    string         // 对应节点
	Config    map[string]any // 实例配置

	Internal  any            // 内部运行时对象 (e.g. *PluginClient)

	Status    InstanceStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// InstanceRegistry 实例注册表接口（唯一事实源）
type InstanceRegistry interface {
	Register(inst *PluginInstance) error
	Start(id string) error
	Stop(id string) error
	Remove(id string) error

	Get(id string) (*PluginInstance, bool)
	ListByRule(ruleID string) []*PluginInstance
	ListAll() []*PluginInstance
}

// InstanceRunner 实例运行器接口（隔离Loader细节）
type InstanceRunner interface {
	Start(inst *PluginInstance) error
	Stop(inst *PluginInstance) error
}
