package engine

import (
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestCollectLogicalRouters(t *testing.T) {
	ruleFlow := &schema.RuleFlow{
		Nodes: []*schema.NodeConfig{
			{
				ID:    "proc_state_event",
				Wires: []string{},
				Routes: []schema.RouteConfig{
					{Label: "redis", To: "sink_state_event_redis"},
					{Label: "events", To: "sink_state_events_ts"},
				},
			},
		},
	}

	routers := collectLogicalRouters(ruleFlow)
	if len(routers) != 1 {
		t.Fatalf("expected 1 logical router, got %d", len(routers))
	}

	got := routers["proc_state_event"]
	if got["redis"] != "sink_state_event_redis" {
		t.Fatalf("unexpected redis route target: %q", got["redis"])
	}
	if got["events"] != "sink_state_events_ts" {
		t.Fatalf("unexpected events route target: %q", got["events"])
	}
}

func TestValidateDAGDetectsCycleViaRoutes(t *testing.T) {
	ruleFlow := &schema.RuleFlow{
		ID: "rule-with-route-cycle",
		Nodes: []*schema.NodeConfig{
			{ID: "source", Plugin: "source-mqtt", Wires: []string{"processor"}},
			{
				ID:     "processor",
				Plugin: "process-status-event",
				Routes: []schema.RouteConfig{{Label: "loop", To: "source"}},
			},
		},
	}

	if err := ValidateDAG(ruleFlow); err == nil {
		t.Fatal("expected cycle detection to include routes, got nil")
	}
}
