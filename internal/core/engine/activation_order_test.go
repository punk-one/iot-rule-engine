package engine

import (
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"testing"
)

func TestStartupRegistersDownstreamBeforeAnyEmittingNode(t *testing.T) {
	rule := &schema.RuleFlow{ID: "rule", Nodes: []*schema.NodeConfig{
		{ID: "source", Type: "source", Wires: []string{"buffer", "direct"}},
		{ID: "buffer", Type: "processor", Wires: []string{"sink"}},
		{ID: "sink", Type: "sink"}, {ID: "direct", Type: "sink"},
	}}
	ordered := downstreamFirstInstances(rule, BuildPluginInstances(rule))
	positions := map[string]int{}
	for i, inst := range ordered {
		positions[inst.NodeID] = i
	}
	if len(positions) != 4 || positions["sink"] >= positions["buffer"] || positions["buffer"] >= positions["source"] || positions["direct"] >= positions["source"] {
		t.Fatalf("unsafe startup order: %v", positions)
	}
}
