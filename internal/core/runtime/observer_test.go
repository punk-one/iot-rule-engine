package runtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestRuntimeObserverRecordsReceiveAndDonePhases(t *testing.T) {
	manager := NewManager()
	instance, err := manager.CreateRuleInstance("rule-test", "1")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	if err := manager.AddNodeInstance(instance.ID, &NodeInstance{
		ID:        instance.ID + "-node_sink",
		NodeID:    "node_sink",
		PluginID:  "sink",
		Version:   "1",
		Status:    NodeStatusRunning,
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add node instance: %v", err)
	}

	observer := NewRuntimeObserver(instance.ID, manager, true)
	eventTime := time.UnixMilli(1773757240267)
	event := &sdk.Event{
		ID:        "evt_123",
		Timestamp: eventTime,
		Payload: map[string]interface{}{
			"device_code": "acm006",
			"time":        int64(1773757240267),
		},
		Metadata: map[string]string{
			"trace_id": "trace-123",
		},
	}

	observation := testObservation("rule-test", "", "node_sink", event)
	observer.OnNodeReceive(observation)
	observer.OnTaskDone(observation, nil)

	events, err := manager.GetTraceEvents(instance.ID, 10, "node_sink")
	if err != nil {
		t.Fatalf("get trace events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 trace events, got %d", len(events))
	}

	phases := map[string]bool{}
	for _, trace := range events {
		phases[trace.Phase] = true
	}
	if !phases["receive"] {
		t.Fatalf("expected trace phases to include receive, got %#v", phases)
	}
	if !phases["done"] {
		t.Fatalf("expected trace phases to include done, got %#v", phases)
	}

	for i, trace := range events {
		if !trace.EventTimestamp.Equal(eventTime) {
			t.Fatalf("trace %d event_timestamp mismatch: got %v want %v", i, trace.EventTimestamp, eventTime)
		}
		if trace.ObservedAt.IsZero() {
			t.Fatalf("trace %d observed_at should be set", i)
		}
		if !trace.Timestamp.Equal(trace.ObservedAt) {
			t.Fatalf("trace %d timestamp should match observed_at", i)
		}
	}
}

func TestRuntimeObserverDoesNotFallbackToEventIDWhenTraceMissing(t *testing.T) {
	manager := NewManager()
	instance, err := manager.CreateRuleInstance("rule-test", "1")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	if err := manager.AddNodeInstance(instance.ID, &NodeInstance{
		ID:        instance.ID + "-node_sink",
		NodeID:    "node_sink",
		PluginID:  "sink",
		Version:   "1",
		Status:    NodeStatusRunning,
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add node instance: %v", err)
	}

	observer := NewRuntimeObserver(instance.ID, manager, true)
	event := &sdk.Event{
		ID:        "evt_missing_trace",
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"value": 1},
	}

	observer.OnNodeReceive(testObservation("rule-test", "", "node_sink", event))

	events, err := manager.GetTraceEvents(instance.ID, 10, "node_sink")
	if err != nil {
		t.Fatalf("get trace events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no trace events when trace_id is missing, got %d", len(events))
	}
}

func TestRuntimeObserverPreservesFullPayloadMessage(t *testing.T) {
	manager := NewManager()
	instance, err := manager.CreateRuleInstance("rule-test", "1")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	if err := manager.AddNodeInstance(instance.ID, &NodeInstance{
		ID:        instance.ID + "-node_sink",
		NodeID:    "node_sink",
		PluginID:  "sink",
		Version:   "1",
		Status:    NodeStatusRunning,
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add node instance: %v", err)
	}

	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"air_lo_alm":   false,
			"gas_flow_rt":  14.80942,
			"gas_hr_sum":   8000.3564,
			"hi_press_alm": false,
			"lo_press_alm": false,
			"pwr_hr_sum":   3112.4,
			"stf_atm_tmp":  779,
			"stf_met_tmp":  745,
			"tap_ratio_tm": 0,
			"tap_temp":     733,
		},
		"device_code": "ZH_JZL_0001",
		"time":        int64(1776150552946),
	}
	wantBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	observer := NewRuntimeObserver(instance.ID, manager, true)
	event := &sdk.Event{
		ID:        "evt_long_message",
		Timestamp: time.UnixMilli(1776150552946),
		Payload:   payload,
		Metadata: map[string]string{
			"trace_id": "trace-long-message",
		},
	}

	observer.OnTaskDone(testObservation("rule-test", "", "node_sink", event), nil)

	events, err := manager.GetTraceEvents(instance.ID, 10, "node_sink")
	if err != nil {
		t.Fatalf("get trace events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 trace event, got %d", len(events))
	}

	if events[0].Message != string(wantBytes) {
		t.Fatalf("expected full payload message, got %q", events[0].Message)
	}
	if strings.HasSuffix(events[0].Message, "...") {
		t.Fatalf("trace message should not be truncated: %q", events[0].Message)
	}
}

func TestRuntimeObserverDisabledSkipsTraceButKeepsMetrics(t *testing.T) {
	manager := NewManager()
	instance, err := manager.CreateRuleInstance("rule-test", "1")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	nodeInstance := &NodeInstance{
		ID:        instance.ID + "-node_sink",
		NodeID:    "node_sink",
		PluginID:  "sink",
		Version:   "1",
		Status:    NodeStatusRunning,
		StartedAt: time.Now(),
	}
	if err := manager.AddNodeInstance(instance.ID, nodeInstance); err != nil {
		t.Fatalf("add node instance: %v", err)
	}

	observer := NewRuntimeObserver(instance.ID, manager, false)
	event := &sdk.Event{
		ID:        "evt_debug_off",
		Timestamp: time.Now(),
		Payload:   map[string]interface{}{"value": 7},
		Metadata: map[string]string{
			"trace_id": "trace-debug-off",
		},
	}

	observation := testObservation("rule-test", "", "node_sink", event)
	observer.OnNodeReceive(observation)
	observer.OnTaskDone(observation, nil)

	events, err := manager.GetTraceEvents(instance.ID, 10, "node_sink")
	if err != nil {
		t.Fatalf("get trace events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no trace events when debug mode is disabled, got %d", len(events))
	}

	gotInstance, err := manager.GetRuleInstance(instance.ID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	gotNode := gotInstance.Nodes[nodeInstance.ID]
	if gotNode == nil {
		t.Fatalf("expected node instance %s to exist", nodeInstance.ID)
	}
	if gotNode.Metrics.EventsIn != 1 {
		t.Fatalf("expected events_in to stay recorded, got %d", gotNode.Metrics.EventsIn)
	}
}

func testObservation(ruleID, fromNodeID, toNodeID string, event *sdk.Event) dispatcher.EventObservation {
	owned := event.Clone()
	return dispatcher.EventObservation{
		RuleID:         ruleID,
		FromNodeID:     fromNodeID,
		ToNodeID:       toNodeID,
		EventID:        owned.ID,
		TraceID:        sdk.GetTraceID(owned),
		EventTimestamp: owned.Timestamp,
		StartedAt:      time.Now().UTC(),
		CompletedAt:    time.Now().UTC(),
		Payload:        owned.Payload,
	}
}
