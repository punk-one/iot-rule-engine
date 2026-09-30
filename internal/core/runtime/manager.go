package runtime

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

// Manager 管理所有规则运行实例
type Manager struct {
	instances           map[string]*RuleInstance
	traceEvents         map[string][]*TraceEvent            // key: instanceID, value: trace events
	customMetrics       map[string]map[string]*CustomMetric // key: instanceID, metricKey -> metric
	mutex               sync.RWMutex
	maxTraceEvents      int // 每个实例最多保留的 Trace 事件数
	maxNodeErrorHistory int // 每个节点最多保留的错误历史条数
}

type ManagerOptions struct {
	MaxTraceEvents      int
	MaxNodeErrorHistory int
}

const (
	defaultMaxTraceEvents      = 1000
	defaultMaxNodeErrorHistory = 100
)

// NewManager 创建运行实例管理器
func NewManager() *Manager {
	return NewManagerWithOptions(ManagerOptions{})
}

// NewManagerWithOptions 使用指定选项创建运行实例管理器
func NewManagerWithOptions(opts ManagerOptions) *Manager {
	maxTraceEvents := opts.MaxTraceEvents
	if maxTraceEvents <= 0 {
		maxTraceEvents = defaultMaxTraceEvents
	}

	maxNodeErrorHistory := opts.MaxNodeErrorHistory
	if maxNodeErrorHistory <= 0 {
		maxNodeErrorHistory = defaultMaxNodeErrorHistory
	}

	return &Manager{
		instances:           make(map[string]*RuleInstance),
		traceEvents:         make(map[string][]*TraceEvent),
		customMetrics:       make(map[string]map[string]*CustomMetric),
		maxTraceEvents:      maxTraceEvents,
		maxNodeErrorHistory: maxNodeErrorHistory,
	}
}

// CreateRuleInstance 创建规则运行实例
func (m *Manager) CreateRuleInstance(ruleID, version string) (*RuleInstance, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instanceID := uuid.New().String()
	now := time.Now()

	instance := &RuleInstance{
		ID:        instanceID,
		RuleID:    ruleID,
		Version:   version,
		Status:    RuleStatusCreated,
		CreatedAt: now,
		Nodes:     make(map[string]*NodeInstance),
		Edges:     make(map[string]*EdgeInstance),
		Metrics:   RuleMetrics{},
	}

	m.instances[instanceID] = instance
	return instance, nil
}

// GetRuleInstance 获取规则运行实例
func (m *Manager) GetRuleInstance(instanceID string) (*RuleInstance, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return nil, fmt.Errorf("rule instance %s not found", instanceID)
	}

	return instance, nil
}

// GetRuleInstanceByRuleID 根据规则ID获取运行实例
func (m *Manager) GetRuleInstanceByRuleID(ruleID string) (*RuleInstance, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, instance := range m.instances {
		if instance.RuleID == ruleID && instance.Status == RuleStatusRunning {
			return instance, nil
		}
	}

	return nil, fmt.Errorf("no running instance found for rule %s", ruleID)
}

// StartRuleInstance 启动规则运行实例
func (m *Manager) StartRuleInstance(instanceID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	if instance.Status != RuleStatusCreated {
		return fmt.Errorf("rule instance %s is not in created status", instanceID)
	}

	instance.Status = RuleStatusRunning
	instance.StartedAt = time.Now()
	return nil
}

// StopRuleInstance 停止规则运行实例
func (m *Manager) StopRuleInstance(instanceID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	now := time.Now()
	instance.Status = RuleStatusStopped
	instance.StoppedAt = &now

	// 停止所有节点
	for _, node := range instance.Nodes {
		node.Status = NodeStatusStopped
		node.Health.Status = sdk.HealthStopped
		node.Health.Code = "SYS_STOPPED"
		node.Health.Message = "node stopped"
		node.Health.ObservedAt = now
		node.Health.UpdatedAt = now
		node.Health.LastHeartbeatAt = &now
	}

	return nil
}

// AddNodeInstance 添加节点实例
func (m *Manager) AddNodeInstance(instanceID string, node *NodeInstance) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	if node.Health.Status == "" {
		node.Health.Status = sdk.HealthUnknown
	}
	instance.Nodes[node.ID] = node
	return nil
}

// AddEdgeInstance 添加边实例
func (m *Manager) AddEdgeInstance(instanceID string, edge *EdgeInstance) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	instance.Edges[edge.ID] = edge
	return nil
}

// UpdateNodeStatus 更新节点状态
func (m *Manager) UpdateNodeStatus(instanceID, nodeID string, status NodeStatus) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	node, exists := instance.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in instance %s", nodeID, instanceID)
	}

	node.Status = status
	return nil
}

func (m *Manager) ApplyNodeHealthUpdate(instanceID, nodeID string, update sdk.ReportHealthArgs) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	node, exists := instance.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in instance %s", nodeID, instanceID)
	}

	observedAt := time.Now()
	if update.ObservedAtUnixNano > 0 {
		observedAt = time.Unix(0, update.ObservedAtUnixNano)
	}

	if !node.Health.ObservedAt.IsZero() {
		if node.Health.ObservedAt.After(observedAt) {
			return nil
		}
		if node.Health.ObservedAt.Equal(observedAt) && update.Sequence > 0 && update.Sequence <= node.Health.Sequence {
			return nil
		}
	}

	node.Health.Sequence = update.Sequence
	node.Health.ObservedAt = observedAt
	node.Health.UpdatedAt = observedAt

	switch update.Kind {
	case sdk.HealthKindHeartbeat:
		node.Health.LastHeartbeatAt = &observedAt
		if update.Message != "" {
			node.Health.Message = update.Message
		}
		if update.Details != nil {
			node.Health.Details = cloneNodeHealthDetails(update.Details)
		}
		return nil
	default:
		status := update.Status
		if status == "" {
			status = sdk.HealthUnknown
		}
		node.Health.Status = status
		node.Health.Code = update.Code
		node.Health.Message = update.Message
		node.Health.Details = cloneNodeHealthDetails(update.Details)
		node.Health.LastHeartbeatAt = &observedAt
		if status == sdk.HealthHealthy {
			node.Health.LastHealthyAt = &observedAt
		}
		m.applyNodeStatusFromHealth(node, status, observedAt, update.Message)
		return nil
	}
}

func cloneNodeHealthDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]string, len(details))
	for key, value := range details {
		out[key] = value
	}
	return out
}

func (m *Manager) applyNodeStatusFromHealth(node *NodeInstance, status sdk.HealthStatus, observedAt time.Time, message string) {
	if node == nil {
		return
	}

	switch status {
	case sdk.HealthInitializing, sdk.HealthUnknown:
		if node.Status == "" {
			node.Status = NodeStatusInit
		}
	case sdk.HealthHealthy, sdk.HealthRecovering:
		if node.Status != NodeStatusStopped {
			node.Status = NodeStatusRunning
		}
	case sdk.HealthDegraded, sdk.HealthUnhealthy:
		if node.Status != NodeStatusStopped {
			node.Status = NodeStatusError
		}
		if strings.TrimSpace(message) != "" {
			m.appendNodeError(node, &NodeError{
				Time:    observedAt,
				Message: message,
			})
		}
	case sdk.HealthStopped:
		node.Status = NodeStatusStopped
	}
}

// RecordNodeError 记录节点错误
func (m *Manager) RecordNodeError(instanceID, nodeID, eventID, message string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	node, exists := instance.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in instance %s", nodeID, instanceID)
	}

	now := time.Now()
	m.appendNodeError(node, &NodeError{
		Time:    now,
		Message: message,
		EventID: eventID,
	})
	node.Status = NodeStatusError
	node.Health.Status = sdk.HealthUnhealthy
	node.Health.Code = "SYS_RUNTIME_ERROR"
	node.Health.Message = message
	node.Health.ObservedAt = now
	node.Health.UpdatedAt = now
	node.Health.LastHeartbeatAt = &now
	node.Health.Sequence++
	instance.Metrics.EventsFailed++

	return nil
}

func (m *Manager) appendNodeError(node *NodeInstance, entry *NodeError) {
	if node == nil || entry == nil {
		return
	}

	node.LastError = entry
	node.ErrorHistory = append(node.ErrorHistory, entry)

	if m.maxNodeErrorHistory > 0 && len(node.ErrorHistory) > m.maxNodeErrorHistory {
		node.ErrorHistory = node.ErrorHistory[len(node.ErrorHistory)-m.maxNodeErrorHistory:]
	}
}

// IncrementNodeMetrics 增加节点指标
func (m *Manager) IncrementNodeMetrics(instanceID, nodeID string, eventsIn, eventsOut, eventsFailed int64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	node, exists := instance.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found in instance %s", nodeID, instanceID)
	}

	node.Metrics.EventsIn += eventsIn
	node.Metrics.EventsOut += eventsOut
	node.Metrics.EventsFailed += eventsFailed
	now := time.Now()
	node.Metrics.LastEventAt = &now

	// 更新规则指标
	instance.Metrics.EventsIn += eventsIn
	instance.Metrics.EventsOut += eventsOut
	instance.Metrics.EventsFailed += eventsFailed
	instance.Metrics.LastEventAt = &now

	return nil
}

// IncrementEdgeMetrics 增加边指标
func (m *Manager) IncrementEdgeMetrics(instanceID, edgeID string, published, consumed, dropped int64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	edge, exists := instance.Edges[edgeID]
	if !exists {
		return fmt.Errorf("edge %s not found in instance %s", edgeID, instanceID)
	}

	edge.Metrics.EventsPublished += published
	edge.Metrics.EventsConsumed += consumed
	edge.Metrics.EventsDropped += dropped
	now := time.Now()
	edge.Metrics.LastEventAt = &now

	return nil
}

// ListRuleInstances 列出所有规则运行实例
func (m *Manager) ListRuleInstances() []*RuleInstance {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	instances := make([]*RuleInstance, 0, len(m.instances))
	for _, instance := range m.instances {
		instances = append(instances, instance)
	}

	return instances
}

// DeleteRuleInstance 删除规则运行实例
func (m *Manager) DeleteRuleInstance(instanceID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	_, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	delete(m.instances, instanceID)
	delete(m.traceEvents, instanceID) // 同时删除 Trace 事件
	delete(m.customMetrics, instanceID)
	return nil
}

// AddTraceEvent 添加 Trace 事件
func (m *Manager) AddTraceEvent(instanceID string, event *TraceEvent) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	_, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	// 获取或创建事件列表
	events, exists := m.traceEvents[instanceID]
	if !exists {
		events = make([]*TraceEvent, 0, m.maxTraceEvents)
	}

	// 添加新事件
	events = append(events, event)

	// 限制事件数量（FIFO）
	if len(events) > m.maxTraceEvents {
		events = events[len(events)-m.maxTraceEvents:]
	}

	m.traceEvents[instanceID] = events
	return nil
}

// GetTraceEvents 获取 Trace 事件列表
// limit: 最多返回的事件数量，0 表示返回所有
// nodeID: 过滤节点 ID，空字符串表示不过滤
func (m *Manager) GetTraceEvents(instanceID string, limit int, nodeID string) ([]*TraceEvent, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.instances[instanceID]
	if !exists {
		return nil, fmt.Errorf("rule instance %s not found", instanceID)
	}

	events, exists := m.traceEvents[instanceID]
	if !exists || len(events) == 0 {
		return []*TraceEvent{}, nil
	}

	// 过滤节点
	var filteredEvents []*TraceEvent
	if nodeID != "" {
		for _, event := range events {
			if event.NodeID == nodeID {
				filteredEvents = append(filteredEvents, event)
			}
		}
	} else {
		filteredEvents = events
	}

	// 限制数量（返回最新的）
	if limit > 0 && len(filteredEvents) > limit {
		filteredEvents = filteredEvents[len(filteredEvents)-limit:]
	}

	// 反转顺序，最新的在前面
	result := make([]*TraceEvent, len(filteredEvents))
	for i, event := range filteredEvents {
		result[len(filteredEvents)-1-i] = event
	}

	return result, nil
}

// RecordCustomMetric records one custom metric sample.
func (m *Manager) RecordCustomMetric(instanceID, ruleID, nodeID, name string, value float64, labels map[string]string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	_, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("rule instance %s not found", instanceID)
	}

	metrics, exists := m.customMetrics[instanceID]
	if !exists {
		metrics = make(map[string]*CustomMetric)
		m.customMetrics[instanceID] = metrics
	}

	metricKey := buildMetricKey(ruleID, nodeID, name, labels)
	metric, exists := metrics[metricKey]
	if !exists {
		labelCopy := make(map[string]string, len(labels))
		for k, v := range labels {
			labelCopy[k] = v
		}
		metric = &CustomMetric{
			Key:    metricKey,
			RuleID: ruleID,
			NodeID: nodeID,
			Name:   name,
			Labels: labelCopy,
		}
		metrics[metricKey] = metric
	}

	metric.Count++
	metric.Sum += value
	metric.LastValue = value
	metric.UpdatedAt = time.Now()
	return nil
}

// GetCustomMetrics lists custom metrics with optional filters.
func (m *Manager) GetCustomMetrics(instanceID, ruleID, nodeID, name string, limit int) ([]*CustomMetric, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.instances[instanceID]
	if !exists {
		return nil, fmt.Errorf("rule instance %s not found", instanceID)
	}

	metricsMap, exists := m.customMetrics[instanceID]
	if !exists {
		return []*CustomMetric{}, nil
	}

	metrics := make([]*CustomMetric, 0, len(metricsMap))
	for _, metric := range metricsMap {
		if ruleID != "" && metric.RuleID != ruleID {
			continue
		}
		if nodeID != "" && metric.NodeID != nodeID {
			continue
		}
		if name != "" && metric.Name != name {
			continue
		}
		metricCopy := *metric
		metricCopy.Labels = make(map[string]string, len(metric.Labels))
		for k, v := range metric.Labels {
			metricCopy.Labels[k] = v
		}
		metrics = append(metrics, &metricCopy)
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].UpdatedAt.After(metrics[j].UpdatedAt)
	})

	if limit > 0 && len(metrics) > limit {
		metrics = metrics[:limit]
	}
	return metrics, nil
}

func buildMetricKey(ruleID, nodeID, name string, labels map[string]string) string {
	parts := []string{ruleID, nodeID, name}
	labelKeys := make([]string, 0, len(labels))
	for key := range labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)
	for _, key := range labelKeys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "|")
}
