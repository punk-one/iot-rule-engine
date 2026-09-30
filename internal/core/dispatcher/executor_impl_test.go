package dispatcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	dispatcherclient "github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/router"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

type fakePluginClient struct {
	info           sdk.PluginInfo
	sendEventCount int
	sendBatchCount int
	lastBatchSize  int
	sendEventErr   error
	sendBatchErr   error
}

func (f *fakePluginClient) Info() sdk.PluginInfo {
	return f.info
}

func (f *fakePluginClient) Start() error { return nil }
func (f *fakePluginClient) Stop() error  { return nil }

func (f *fakePluginClient) SendEvent(e sdk.Event) error {
	f.sendEventCount++
	return f.sendEventErr
}

func (f *fakePluginClient) SendEvents(events []sdk.Event) error {
	f.sendBatchCount++
	f.lastBatchSize = len(events)
	return f.sendBatchErr
}

type fakeScheduler struct {
	tasks []*scheduler.DispatchTask
}

func (f *fakeScheduler) Submit(task *scheduler.DispatchTask) error {
	f.tasks = append(f.tasks, task)
	return nil
}

func (f *fakeScheduler) Start() error { return nil }
func (f *fakeScheduler) Stop() error  { return nil }

type queueFullScheduler struct{}

func (q *queueFullScheduler) Submit(task *scheduler.DispatchTask) error {
	return scheduler.ErrQueueFull
}

func (q *queueFullScheduler) Start() error { return nil }
func (q *queueFullScheduler) Stop() error  { return nil }

func TestExecuteTaskUsesBatchWhenPluginSupportsIt(t *testing.T) {
	plugin := &fakePluginClient{
		info: sdk.PluginInfo{
			Capabilities: sdk.PluginCapabilities{SupportBatch: true},
		},
	}

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
		Events: []sdk.Event{
			{ID: "e1"},
			{ID: "e2"},
		},
	}

	if err := d.executeTask(task); err != nil {
		t.Fatalf("executeTask failed: %v", err)
	}

	if plugin.sendBatchCount != 1 {
		t.Fatalf("expected SendEvents once, got %d", plugin.sendBatchCount)
	}
	if plugin.lastBatchSize != 2 {
		t.Fatalf("expected batch size 2, got %d", plugin.lastBatchSize)
	}
	if plugin.sendEventCount != 0 {
		t.Fatalf("expected SendEvent not called, got %d", plugin.sendEventCount)
	}
}

func TestExecuteTaskFallsBackToSingleWhenBatchUnsupported(t *testing.T) {
	plugin := &fakePluginClient{
		info: sdk.PluginInfo{
			Capabilities: sdk.PluginCapabilities{SupportBatch: false},
		},
	}

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
		Events: []sdk.Event{
			{ID: "e1"},
			{ID: "e2"},
		},
	}

	if err := d.executeTask(task); err != nil {
		t.Fatalf("executeTask failed: %v", err)
	}

	if plugin.sendEventCount != 2 {
		t.Fatalf("expected SendEvent called twice, got %d", plugin.sendEventCount)
	}
	if plugin.sendBatchCount != 0 {
		t.Fatalf("expected SendEvents not called, got %d", plugin.sendBatchCount)
	}
}

func TestOnPluginEmitBatchGroupsTasksByDownstreamNode(t *testing.T) {
	sched := &fakeScheduler{}
	d := &Dispatcher{
		ruleID:           RuleID("rule-1"),
		ctx:              context.Background(),
		router:           router.NewRouter(),
		scheduler:        sched,
		rules:            map[string]*RuleRuntime{"rule-1": {RuleID: "rule-1", Edges: map[string][]string{"node-1": {"node-2", "node-3"}}}},
		overflowStrategy: "reject",
	}

	events := []sdk.Event{
		{ID: "e1", Payload: map[string]interface{}{"v": 1}, Metadata: map[string]string{"trace_id": "trace-e1"}},
		{ID: "e2", Payload: map[string]interface{}{"v": 2}, Metadata: map[string]string{"trace_id": "trace-e2"}},
	}

	if err := d.OnPluginEmitBatch("rule-1", "node-1", events); err != nil {
		t.Fatalf("OnPluginEmitBatch failed: %v", err)
	}

	if len(sched.tasks) != 2 {
		t.Fatalf("expected 2 grouped tasks, got %d", len(sched.tasks))
	}

	for _, task := range sched.tasks {
		if len(task.Events) != 2 {
			t.Fatalf("expected grouped task size 2, got %d", len(task.Events))
		}
	}
}

func TestOnPluginEmitGeneratesTraceIDWhenPluginDropsMetadata(t *testing.T) {
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
		ID:      "e1",
		Payload: map[string]interface{}{"v": 1},
	}

	if err := d.OnPluginEmit("rule-1", "node-1", event); err != nil {
		t.Fatalf("OnPluginEmit failed: %v", err)
	}

	if got := sdk.GetTraceID(*event); got != "" {
		t.Fatal("dispatcher must not mutate the caller-owned event")
	}
	if len(sched.tasks) != 1 {
		t.Fatalf("expected 1 scheduled task, got %d", len(sched.tasks))
	}
	if got := sdk.GetTraceID(sched.tasks[0].Event); got == "" {
		t.Fatal("expected scheduled event to carry trace_id")
	}
}

func TestExecuteTaskBatchFailurePublishesDLQPerEventAndHooks(t *testing.T) {
	oldPublishRuleDLQ := publishRuleDLQ
	defer func() { publishRuleDLQ = oldPublishRuleDLQ }()

	var dlqCount int
	publishRuleDLQ = func(ruleID, nodeID string, data []byte) error {
		dlqCount++
		return nil
	}

	plugin := &fakePluginClient{
		info: sdk.PluginInfo{
			Capabilities: sdk.PluginCapabilities{SupportBatch: true},
		},
		sendBatchErr: errors.New("batch failed"),
	}

	var onReceiveCount int
	var onDoneCount int
	var errorCount int

	d := &Dispatcher{
		ctx:          context.Background(),
		eventTimeout: time.Second,
		errorHandler: func(nodeID, eventID string, err error) {
			errorCount++
		},
		hooks: DispatcherHooks{
			OnNodeReceive: func(EventObservation) {
				onReceiveCount++
			},
			OnTaskDone: func(EventObservation, error) {
				onDoneCount++
			},
		},
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
		Events: []sdk.Event{
			{ID: "e1"},
			{ID: "e2"},
		},
	}

	if err := d.executeTask(task); err == nil {
		t.Fatal("expected executeTask to fail, got nil")
	}

	if dlqCount != 2 {
		t.Fatalf("expected 2 DLQ publications, got %d", dlqCount)
	}
	if onReceiveCount != 2 {
		t.Fatalf("expected 2 OnNodeReceive calls, got %d", onReceiveCount)
	}
	if onDoneCount != 2 {
		t.Fatalf("expected 2 OnTaskDone calls, got %d", onDoneCount)
	}
	if errorCount != 2 {
		t.Fatalf("expected 2 errorHandler calls, got %d", errorCount)
	}
}

func TestOnPluginEmitBatchBackpressureCallsMetricAndErrorHandler(t *testing.T) {
	var metricName string
	var metricValue float64
	var metricLabels map[string]string
	var metricCount int
	var emitCount int
	var errorCount int

	d := &Dispatcher{
		ruleID:           RuleID("rule-1"),
		ctx:              context.Background(),
		router:           router.NewRouter(),
		scheduler:        &queueFullScheduler{},
		overflowStrategy: "reject",
		errorHandler: func(nodeID, eventID string, err error) {
			errorCount++
		},
		hooks: DispatcherHooks{
			OnNodeEmit: func(EventObservation) {
				emitCount++
			},
			OnNodeMetric: func(ruleID, nodeID string, name string, value float64, labels map[string]string) {
				metricCount++
				metricName = name
				metricValue = value
				metricLabels = labels
			},
		},
		rules: map[string]*RuleRuntime{
			"rule-1": {
				RuleID: "rule-1",
				Edges: map[string][]string{
					"node-1": {"node-2"},
				},
			},
		},
	}

	events := []sdk.Event{
		{ID: "e1", Payload: map[string]interface{}{"v": 1}, Metadata: map[string]string{"trace_id": "trace-e1"}},
		{ID: "e2", Payload: map[string]interface{}{"v": 2}, Metadata: map[string]string{"trace_id": "trace-e2"}},
	}

	err := d.OnPluginEmitBatch("rule-1", "node-1", events)
	if err == nil {
		t.Fatal("expected backpressure error, got nil")
	}
	if !strings.Contains(err.Error(), "2/2") {
		t.Fatalf("expected failure count in error, got: %v", err)
	}

	if emitCount != 2 {
		t.Fatalf("expected OnNodeEmit to be called for each event, got %d", emitCount)
	}
	if errorCount != 2 {
		t.Fatalf("expected errorHandler called twice, got %d", errorCount)
	}
	if metricCount != 1 {
		t.Fatalf("expected one metric report, got %d", metricCount)
	}
	if metricName != "dispatcher.backpressure" {
		t.Fatalf("unexpected metric name: %s", metricName)
	}
	if metricValue != 2 {
		t.Fatalf("expected metric value 2, got %v", metricValue)
	}
	if metricLabels["to_node"] != "node-2" {
		t.Fatalf("unexpected metric label to_node: %+v", metricLabels)
	}
}
