package scheduler

import (
	"context"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// DispatchTask represents a task to be executed by the scheduler
// Standardized task unit
type DispatchTask struct {
	RuleID   string
	FromNode string
	ToNode   string
	Event    sdk.Event
	Events   []sdk.Event
	TraceCtx context.Context
}

// Clone creates a scheduler-owned task. Every Event and batch entry receives
// independent mutable Payload and Metadata containers.
func (t *DispatchTask) Clone() *DispatchTask {
	if t == nil {
		return nil
	}
	cloned := *t
	cloned.Event = t.Event.Clone()
	if t.Events != nil {
		cloned.Events = make([]sdk.Event, len(t.Events))
		for i := range t.Events {
			cloned.Events[i] = t.Events[i].Clone()
		}
	}
	return &cloned
}
