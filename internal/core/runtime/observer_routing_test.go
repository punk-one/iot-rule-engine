package runtime

import (
	"testing"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
)

func TestRuntimeObserverUsesExplicitRouteForEdgeMetrics(t *testing.T) {
	manager := NewManager()
	instance, err := manager.CreateRuleInstance("rule-test", "1")
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	nodeID := instance.ID + "-node_sink"
	if err := manager.AddNodeInstance(instance.ID, &NodeInstance{
		ID:        nodeID,
		NodeID:    "node_sink",
		PluginID:  "sink",
		Version:   "1",
		Status:    NodeStatusRunning,
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("add node instance: %v", err)
	}
	edgeID := instance.ID + "-node_source-node_sink"
	if err := manager.AddEdgeInstance(instance.ID, &EdgeInstance{
		ID:       edgeID,
		EdgeID:   "edge-1",
		FromNode: "node_source",
		ToNode:   "node_sink",
	}); err != nil {
		t.Fatalf("add edge instance: %v", err)
	}

	observer := NewRuntimeObserver(instance.ID, manager, false)
	observer.OnNodeReceive(dispatcher.EventObservation{
		RuleID:     "rule-test",
		FromNodeID: "node_source",
		ToNodeID:   "node_sink",
		EventID:    "event-1",
	})

	got, err := manager.GetRuleInstance(instance.ID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	if got.Edges[edgeID].Metrics.EventsConsumed != 1 {
		t.Fatalf("expected explicit edge consumed metric to be 1, got %d", got.Edges[edgeID].Metrics.EventsConsumed)
	}
}
