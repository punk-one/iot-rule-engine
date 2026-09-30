package dispatcher

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	"github.com/punk-one/iot-rule-engine/internal/core/dlq"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

var publishRuleDLQ = bus.PublishRuleDLQ

func (d *Dispatcher) publishDLQ(task *scheduler.DispatchTask, err error) {
	if task == nil || err == nil {
		return
	}

	d.publishDLQEvent(task.RuleID, task.FromNode, task.ToNode, task.Event, err)
}

func (d *Dispatcher) publishDLQEvent(ruleID, fromNodeID, toNodeID string, event sdk.Event, err error) {
	if err == nil {
		return
	}

	traceID := sdk.GetTraceID(event)

	msg := dlq.Message{
		ID:         uuid.New().String(),
		RuleID:     ruleID,
		FromNodeID: fromNodeID,
		NodeID:     toNodeID,
		Event:      event,
		Error:      err.Error(),
		FailedAt:   time.Now(),
		Attempt:    1,
		TraceID:    traceID,
	}
	data, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		pkglog.Error("Failed to marshal DLQ message (rule=%s, node=%s): %v", ruleID, toNodeID, marshalErr)
		return
	}

	if pubErr := publishRuleDLQ(ruleID, toNodeID, data); pubErr != nil {
		pkglog.Error("Failed to publish DLQ message (rule=%s, node=%s, event=%s): %v", ruleID, toNodeID, event.ID, pubErr)
	}
}

// ReplayEventToNode replays a DLQ event back to a target node.
func (d *Dispatcher) ReplayEventToNode(ruleID, fromNodeID, toNodeID string, event sdk.Event) error {
	if !d.started {
		return ErrDispatcherNotStarted
	}

	if ruleID == "" || toNodeID == "" {
		return fmt.Errorf("ruleID and toNodeID are required for replay")
	}
	if fromNodeID == "" {
		fromNodeID = "dlq_replay"
	}

	owned := event.Clone()
	owned.RuleID = ruleID
	owned.Source = fromNodeID

	// A replay is effectively a re-emit from the original source node.
	if d.hooks.OnNodeEmit != nil {
		d.notifyNodeEmit(ruleID, fromNodeID, owned)
	}

	task := &scheduler.DispatchTask{
		RuleID:   ruleID,
		FromNode: fromNodeID,
		ToNode:   toNodeID,
		Event:    owned.Clone(),
		TraceCtx: d.ctx,
	}
	return d.scheduler.Submit(task)
}
