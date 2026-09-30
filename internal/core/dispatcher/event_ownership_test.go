package dispatcher

import (
	"context"
	"sync"
	"testing"
	"time"

	dispatcherclient "github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/router"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestOnPluginEmitCreatesIndependentBranchEvents(t *testing.T) {
	sched := &fakeScheduler{}
	d := newOwnershipTestDispatcher(sched)
	d.hooks = DispatcherHooks{
		CapturePayload: true,
		OnNodeEmit: func(observation EventObservation) {
			observation.Payload["nested"].(map[string]interface{})["value"] = "observer"
		},
	}
	event := &sdk.Event{
		ID: "event-1",
		Payload: map[string]interface{}{
			"nested": map[string]interface{}{"value": "original"},
		},
		Metadata: map[string]string{"trace_id": "trace-1"},
	}

	if err := d.OnPluginEmit("rule-1", "node-1", event); err != nil {
		t.Fatalf("OnPluginEmit failed: %v", err)
	}
	if len(sched.tasks) != 2 {
		t.Fatalf("expected two branch tasks, got %d", len(sched.tasks))
	}
	if event.RuleID != "" || event.Source != "" {
		t.Fatal("dispatcher mutated caller-owned scheduling fields")
	}
	if _, ok := event.Metadata["rule_id"]; ok {
		t.Fatal("rule_id must not be stored in Metadata")
	}
	if _, ok := event.Metadata["node_id"]; ok {
		t.Fatal("node_id must not be stored in Metadata")
	}
	if got := event.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("observer mutated caller payload: %v", got)
	}

	first := &sched.tasks[0].Event
	second := &sched.tasks[1].Event
	first.Metadata["branch"] = "first"
	first.Payload["nested"].(map[string]interface{})["value"] = "first"
	if _, ok := second.Metadata["branch"]; ok {
		t.Fatal("branches share Metadata")
	}
	if got := second.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("branches share nested Payload: %v", got)
	}
	if first.RuleID != "rule-1" || first.Source != "node-1" ||
		second.RuleID != "rule-1" || second.Source != "node-1" {
		t.Fatal("scheduler fields were not carried outside Metadata")
	}
}

func TestOnPluginEmitBatchCreatesIndependentTargetBatches(t *testing.T) {
	sched := &fakeScheduler{}
	d := newOwnershipTestDispatcher(sched)
	events := []sdk.Event{{
		ID:       "event-1",
		Payload:  map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
		Metadata: map[string]string{"trace_id": "trace-1"},
	}}

	if err := d.OnPluginEmitBatch("rule-1", "node-1", events); err != nil {
		t.Fatalf("OnPluginEmitBatch failed: %v", err)
	}
	if len(sched.tasks) != 2 {
		t.Fatalf("expected two target batches, got %d", len(sched.tasks))
	}
	sched.tasks[0].Events[0].Metadata["target"] = "first"
	sched.tasks[0].Events[0].Payload["nested"].(map[string]interface{})["value"] = "first"
	if _, ok := sched.tasks[1].Events[0].Metadata["target"]; ok {
		t.Fatal("target batches share Metadata")
	}
	if got := sched.tasks[1].Events[0].Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("target batches share nested Payload: %v", got)
	}
	if got := events[0].Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("dispatcher mutated caller batch: %v", got)
	}
}

func TestPipelineCreatesIndependentBranchEvents(t *testing.T) {
	sched := &fakeScheduler{}
	d := newOwnershipTestDispatcher(sched)
	pipeline := d.UsePipeline(PipelineConfig{})
	event := &sdk.Event{
		ID:       "event-1",
		Payload:  map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
		Metadata: map[string]string{"trace_id": "trace-1"},
	}

	if err := d.OnPluginEmitWithPipeline(pipeline, "rule-1", "node-1", event); err != nil {
		t.Fatalf("OnPluginEmitWithPipeline failed: %v", err)
	}
	if len(sched.tasks) != 2 {
		t.Fatalf("expected two branch tasks, got %d", len(sched.tasks))
	}
	sched.tasks[0].Event.Metadata["branch"] = "first"
	sched.tasks[0].Event.Payload["nested"].(map[string]interface{})["value"] = "first"
	if _, ok := sched.tasks[1].Event.Metadata["branch"]; ok {
		t.Fatal("pipeline branches share Metadata")
	}
	if got := sched.tasks[1].Event.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("pipeline branches share nested Payload: %v", got)
	}
	if got := event.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("pipeline mutated caller payload: %v", got)
	}
}

func TestBranchEventsCanBeMutatedConcurrently(t *testing.T) {
	sched := &fakeScheduler{}
	d := newOwnershipTestDispatcher(sched)
	event := &sdk.Event{
		ID:       "event-1",
		Payload:  map[string]interface{}{"nested": map[string]interface{}{"value": 0}},
		Metadata: map[string]string{"trace_id": "trace-1"},
	}
	if err := d.OnPluginEmit("rule-1", "node-1", event); err != nil {
		t.Fatalf("OnPluginEmit failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := range sched.tasks {
		branch := &sched.tasks[i].Event
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				branch.Metadata["value"] = "branch"
				branch.Payload["nested"].(map[string]interface{})["value"] = value
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			event.Metadata["value"] = "caller"
			event.Payload["nested"].(map[string]interface{})["value"] = i
		}
	}()
	wg.Wait()
}

func TestPluginCannotMutateSchedulerOwnedEvent(t *testing.T) {
	plugin := mutatingOwnershipPlugin{}
	d := &Dispatcher{
		ctx:          context.Background(),
		eventTimeout: time.Second,
		rules: map[string]*RuleRuntime{
			"rule-1": {
				RuleID: "rule-1",
				Plugins: map[string]dispatcherclient.PluginClient{
					"node-2": plugin,
				},
			},
		},
	}
	task := &scheduler.DispatchTask{
		RuleID:   "rule-1",
		FromNode: "node-1",
		ToNode:   "node-2",
		Event: sdk.Event{
			ID:       "event-1",
			Payload:  map[string]interface{}{"nested": map[string]interface{}{"value": "original"}},
			Metadata: map[string]string{"trace_id": "trace-1"},
		},
	}

	if err := d.executeTask(task); err != nil {
		t.Fatalf("executeTask failed: %v", err)
	}
	if _, ok := task.Event.Metadata["plugin"]; ok {
		t.Fatal("plugin mutated scheduler-owned Metadata")
	}
	if got := task.Event.Payload["nested"].(map[string]interface{})["value"]; got != "original" {
		t.Fatalf("plugin mutated scheduler-owned Payload: %v", got)
	}
}

type mutatingOwnershipPlugin struct{}

func (mutatingOwnershipPlugin) Info() sdk.PluginInfo { return sdk.PluginInfo{} }
func (mutatingOwnershipPlugin) Start() error         { return nil }
func (mutatingOwnershipPlugin) Stop() error          { return nil }
func (mutatingOwnershipPlugin) SendEvent(event sdk.Event) error {
	event.Metadata["plugin"] = "mutated"
	event.Payload["nested"].(map[string]interface{})["value"] = "mutated"
	return nil
}
func (plugin mutatingOwnershipPlugin) SendEvents(events []sdk.Event) error {
	for i := range events {
		if err := plugin.SendEvent(events[i]); err != nil {
			return err
		}
	}
	return nil
}

func newOwnershipTestDispatcher(sched scheduler.Scheduler) *Dispatcher {
	return &Dispatcher{
		ruleID:           RuleID("rule-1"),
		ctx:              context.Background(),
		router:           router.NewRouter(),
		scheduler:        sched,
		rules:            map[string]*RuleRuntime{"rule-1": {RuleID: "rule-1", Edges: map[string][]string{"node-1": {"node-2", "node-3"}}}},
		eventTimeout:     time.Second,
		overflowStrategy: "reject",
	}
}
