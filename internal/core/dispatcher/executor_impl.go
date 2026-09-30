package dispatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// executeTask 是 Scheduler 的回调，执行真正的发送逻辑
func (d *Dispatcher) executeTask(task *scheduler.DispatchTask) error {
	nodeID := task.ToNode
	var client client.PluginClient
	var ok bool

	// 从多规则映射中查找
	d.rulesMu.RLock()
	if rule, exists := d.rules[task.RuleID]; exists {
		client, ok = rule.Plugins[nodeID]
	}
	d.rulesMu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin client not found for node %s in rule %s", nodeID, task.RuleID)
	}

	events := task.Events
	if len(events) == 0 {
		events = []sdk.Event{task.Event}
	}

	startedAt := time.Now().UTC()
	// 通知 Observer 节点接收事件（在发送之前调用，更新 EventsIn）
	if d.hooks.OnNodeReceive != nil {
		for i := range events {
			d.hooks.OnNodeReceive(d.newEventObservation(
				task.RuleID, task.FromNode, nodeID,
				events[i], startedAt, time.Time{},
			))
		}
	}

	sendErr := d.sendEventsWithTimeout(client, events, task.RuleID, nodeID)
	completedAt := time.Now().UTC()

	if sendErr != nil {
		for i := range events {
			d.publishDLQEvent(task.RuleID, task.FromNode, nodeID, events[i], sendErr)
			if d.errorHandler != nil {
				d.errorHandler(nodeID, events[i].ID, sendErr)
			}
		}
	}

	// 通知 Observer 任务执行完成（通过 Hooks）
	if d.hooks.OnTaskDone != nil {
		for i := range events {
			d.hooks.OnTaskDone(d.newEventObservation(
				task.RuleID, task.FromNode, nodeID,
				events[i], startedAt, completedAt,
			), sendErr)
		}
	}

	return sendErr
}

func (d *Dispatcher) sendEventsWithTimeout(client client.PluginClient, events []sdk.Event, ruleID, nodeID string) error {
	// A plugin receives its own copies and cannot mutate scheduler-owned events.
	pluginEvents := make([]sdk.Event, len(events))
	for i := range events {
		pluginEvents[i] = events[i].Clone()
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.eventTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		if len(pluginEvents) > 1 && client.Info().Capabilities.SupportBatch {
			errChan <- client.SendEvents(pluginEvents)
			return
		}

		var firstErr error
		for i := range pluginEvents {
			if err := client.SendEvent(pluginEvents[i]); err != nil {
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		errChan <- firstErr
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("timeout executing task for node %s in rule %s", nodeID, ruleID)
	}
}
