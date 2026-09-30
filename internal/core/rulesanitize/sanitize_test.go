package rulesanitize

import (
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestCleanupVirtualSourceNodes(t *testing.T) {
	rule := &schema.RuleFlow{
		ID: "test-rule",
		Nodes: []*schema.NodeConfig{
			{
				ID:   VirtualSourceNodeID,
				Type: "source-s7",
				Name: "Data Source",
			},
			{
				ID:     "src_s7",
				Type:   "source",
				Plugin: "source-s7",
				Name:   "S7 Source",
			},
			{
				ID:     "sink_mqtt",
				Type:   "sink",
				Plugin: "sink-mqtt",
				Name:   "MQTT Sink",
			},
		},
	}

	CleanupVirtualSourceNodes(rule)

	if len(rule.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after cleanup, got %d", len(rule.Nodes))
	}
	for _, node := range rule.Nodes {
		if node.ID == VirtualSourceNodeID {
			t.Fatalf("virtual source node should have been removed")
		}
	}
}
