package dispatcher

import (
	"time"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// EventObservation is an observer-owned snapshot. Scheduling context is
// explicit and Payload, when requested, is detached from the business Event.
type EventObservation struct {
	RuleID         string
	FromNodeID     string
	ToNodeID       string
	EventID        string
	TraceID        string
	EventTimestamp time.Time
	StartedAt      time.Time
	CompletedAt    time.Time
	Payload        map[string]interface{}
}

// DispatcherHooks Dispatcher 事件钩子
// Dispatcher 通过 Hooks 通知 Observer，而不是直接操作 Runtime
type DispatcherHooks struct {
	// CapturePayload enables detached payload snapshots for debug traces.
	CapturePayload bool

	// OnNodeReceive 当节点接收事件时调用（上游 → 当前节点）
	OnNodeReceive func(EventObservation)

	// OnNodeEmit 当节点发出事件时调用（当前节点 → 下游）
	OnNodeEmit func(EventObservation)

	// OnTaskDone 当任务执行完成时调用（Processor / Sink 执行完成）
	OnTaskDone func(EventObservation, error)

	// OnNodeMetric 当节点报告自定义指标时调用
	OnNodeMetric func(ruleID, nodeID string, name string, value float64, labels map[string]string)

	// OnNodeAck 当节点确认消息时调用
	OnNodeAck func(ruleID, nodeID string, eventID string)
}

func (d *Dispatcher) newEventObservation(
	ruleID, fromNodeID, toNodeID string,
	event sdk.Event,
	startedAt, completedAt time.Time,
) EventObservation {
	observation := EventObservation{
		RuleID:         ruleID,
		FromNodeID:     fromNodeID,
		ToNodeID:       toNodeID,
		EventID:        event.ID,
		TraceID:        sdk.GetTraceID(event),
		EventTimestamp: event.Timestamp,
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
	}
	if d.hooks.CapturePayload {
		observation.Payload = event.Clone().Payload
	}
	return observation
}

func (d *Dispatcher) notifyNodeEmit(ruleID, fromNodeID string, event sdk.Event) {
	if d.hooks.OnNodeEmit == nil {
		return
	}
	d.hooks.OnNodeEmit(d.newEventObservation(
		ruleID,
		fromNodeID,
		"",
		event,
		time.Time{},
		time.Time{},
	))
}
