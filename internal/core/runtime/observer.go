package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// RuntimeObserver Runtime 唯一写入口
// 职责：订阅 ExecutionEvent，更新 Metrics 和 Trace
type RuntimeObserver struct {
	instanceID   string
	runtimeMgr   *Manager
	debugEnabled bool
}

// NewRuntimeObserver 创建 RuntimeObserver
func NewRuntimeObserver(instanceID string, runtimeMgr *Manager, debugEnabled bool) *RuntimeObserver {
	return &RuntimeObserver{
		instanceID:   instanceID,
		runtimeMgr:   runtimeMgr,
		debugEnabled: debugEnabled,
	}
}

// OnNodeReceive 处理节点接收事件（上游 → 当前节点）
func (o *RuntimeObserver) OnNodeReceive(observation dispatcher.EventObservation) {
	ruleID := observation.RuleID
	nodeID := observation.ToNodeID
	execEvent := o.convertObservationToExecutionEvent(observation, nodeID, ExecutionEventTypeExec)

	// 更新节点的 EventsIn
	runtimeNodeID := fmt.Sprintf("%s-%s", o.instanceID, nodeID)
	if err := o.runtimeMgr.IncrementNodeMetrics(o.instanceID, runtimeNodeID, 1, 0, 0); err != nil {
		pkglog.Error("Failed to increment node metrics for node %s (rule: %s): %v", nodeID, ruleID, err)
	}
	if observation.FromNodeID != "" && observation.FromNodeID != "ingress" &&
		observation.FromNodeID != "legacy_nats" && observation.FromNodeID != "dlq_replay" {
		runtimeEdgeID := fmt.Sprintf("%s-%s-%s", o.instanceID, observation.FromNodeID, nodeID)
		if err := o.runtimeMgr.IncrementEdgeMetrics(o.instanceID, runtimeEdgeID, 0, 1, 0); err != nil {
			pkglog.Error("Failed to increment consumed edge metric %s (rule: %s): %v", runtimeEdgeID, ruleID, err)
		}
	}

	// 添加 trace 事件
	if o.debugEnabled && execEvent.TraceID != "" {
		o.addTraceEvent(ruleID, nodeID, execEvent, "receive", "success")
	}
}

// OnNodeEmit 处理节点发出事件（当前节点 → 下游）
func (o *RuntimeObserver) OnNodeEmit(observation dispatcher.EventObservation) {
	ruleID := observation.RuleID
	nodeID := observation.FromNodeID
	execEvent := o.convertObservationToExecutionEvent(observation, nodeID, ExecutionEventTypeOut)

	// 更新节点的 EventsOut
	runtimeNodeID := fmt.Sprintf("%s-%s", o.instanceID, nodeID)
	if err := o.runtimeMgr.IncrementNodeMetrics(o.instanceID, runtimeNodeID, 0, 1, 0); err != nil {
		pkglog.Error("Failed to increment node metrics for node %s (rule: %s): %v", nodeID, ruleID, err)
	}

	// 添加 trace 事件
	if o.debugEnabled && execEvent.TraceID != "" {
		o.addTraceEvent(ruleID, nodeID, execEvent, "emit", "success")
	}
}

// OnTaskDone 处理任务完成事件（Processor / Sink 执行完成）
func (o *RuntimeObserver) OnTaskDone(observation dispatcher.EventObservation, err error) {
	ruleID := observation.RuleID
	nodeID := observation.ToNodeID
	execEvent := o.convertObservationToExecutionEvent(observation, nodeID, ExecutionEventTypeExec)

	if err != nil {
		// 更新错误计数
		runtimeNodeID := fmt.Sprintf("%s-%s", o.instanceID, nodeID)
		if incErr := o.runtimeMgr.IncrementNodeMetrics(o.instanceID, runtimeNodeID, 0, 0, 1); incErr != nil {
			pkglog.Error("Failed to increment error metrics for node %s (rule: %s): %v", nodeID, ruleID, incErr)
		}

		// 添加错误 trace 事件
		execEvent.Error = err.Error()
		if o.debugEnabled && execEvent.TraceID != "" {
			o.addTraceEvent(ruleID, nodeID, execEvent, "done", "failed")
		}
	} else {
		// 任务成功完成，添加 trace 事件
		if o.debugEnabled && execEvent.TraceID != "" {
			o.addTraceEvent(ruleID, nodeID, execEvent, "done", "success")
		}
	}
}

// OnNodeMetric 处理自定义指标
func (o *RuntimeObserver) OnNodeMetric(ruleID, nodeID string, name string, value float64, labels map[string]string) {
	if err := o.runtimeMgr.RecordCustomMetric(o.instanceID, ruleID, nodeID, name, value, labels); err != nil {
		pkglog.Error("Failed to record custom metric for node %s (rule: %s): %v", nodeID, ruleID, err)
		return
	}
	pkglog.Debug("Custom Metric from node %s (rule: %s): %s = %f %v", nodeID, ruleID, name, value, labels)
}

// OnNodeAck 处理消息确认
func (o *RuntimeObserver) OnNodeAck(ruleID, nodeID string, eventID string) {
	pkglog.Debug("Message Ack from node %s (rule: %s): %s", nodeID, ruleID, eventID)
}

// convertObservationToExecutionEvent converts an observer-owned value object.
func (o *RuntimeObserver) convertObservationToExecutionEvent(observation dispatcher.EventObservation, nodeID, eventType string) *ExecutionEvent {
	return &ExecutionEvent{
		RuleID:    observation.RuleID,
		NodeID:    nodeID,
		EventID:   observation.EventID,
		TraceID:   observation.TraceID,
		Type:      eventType,
		Payload:   observation.Payload,
		Timestamp: observation.EventTimestamp,
	}
}

// addTraceEvent 添加 trace 事件（内部方法）
func (o *RuntimeObserver) addTraceEvent(ruleID, nodeID string, event *ExecutionEvent, phase string, status string) {
	observedAt := time.Now()

	// 生成 message 字段
	message := event.Message
	if message == "" && event.Payload != nil {
		if payloadJSON, err := json.Marshal(event.Payload); err == nil {
			message = string(payloadJSON)
		}
	}
	if message == "" {
		message = "Event processed successfully"
	}
	if event.Error != "" {
		message = fmt.Sprintf("Error: %s", event.Error)
	}

	traceEvent := &TraceEvent{
		InstanceID:     o.instanceID,
		TraceID:        event.TraceID,
		RuleID:         ruleID,
		NodeID:         nodeID,
		EventID:        event.EventID,
		Phase:          phase,
		Status:         status,
		Payload:        event.Payload,
		Message:        message,
		Timestamp:      observedAt,
		EventTimestamp: event.Timestamp,
		ObservedAt:     observedAt,
	}

	if err := o.runtimeMgr.AddTraceEvent(o.instanceID, traceEvent); err != nil {
		pkglog.Error("Failed to add trace event for node %s (rule: %s): %v", nodeID, ruleID, err)
	}
}
