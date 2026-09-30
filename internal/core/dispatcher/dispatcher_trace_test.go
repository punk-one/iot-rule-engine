package dispatcher

import (
	"context"
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/router"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestOnPluginEmitBackfillsMissingTraceID(t *testing.T) {
	sched := &fakeScheduler{}
	d := &Dispatcher{
		ruleID:           RuleID("rule-1"),
		ctx:              context.Background(),
		router:           router.NewRouter(),
		scheduler:        sched,
		rules:            map[string]*RuleRuntime{"rule-1": {RuleID: "rule-1", Edges: map[string][]string{"node-1": {"node-2"}}}},
		overflowStrategy: "reject",
	}

	event := &sdk.Event{
		ID:      "evt-1",
		Payload: map[string]interface{}{"value": 1},
	}

	err := d.OnPluginEmit("rule-1", "node-1", event)
	if err != nil {
		t.Fatalf("expected dispatcher to backfill trace_id, got %v", err)
	}
	if got := sdk.GetTraceID(*event); got != "" {
		t.Fatal("dispatcher must not mutate the caller-owned event")
	}
	if len(sched.tasks) != 1 {
		t.Fatalf("expected 1 scheduled task, got %d", len(sched.tasks))
	}
	if got := sdk.GetTraceID(sched.tasks[0].Event); got == "" {
		t.Fatal("expected scheduler-owned event to carry trace_id")
	}
}
