package router

import (
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestResolveWithGrayReleaseDoesNotMutateCallerMetadata(t *testing.T) {
	router := NewRouter()
	router.RegisterGrayConfig(&schema.GrayReleaseConfig{
		RuleID:      "rule-1",
		Enabled:     true,
		Strategy:    "percentage",
		Percentage:  100,
		Version:     2,
		TargetNodes: []string{"node-gray"},
	})
	event := sdk.Event{Metadata: map[string]string{"trace_id": "trace-1"}}

	targets := router.ResolveWithGrayRelease("rule-1", event)
	if len(targets) != 1 || targets[0] != "node-gray" {
		t.Fatalf("unexpected gray targets: %v", targets)
	}
	if _, ok := event.Metadata["gray_version"]; ok {
		t.Fatal("gray routing mutated caller-owned Metadata")
	}
	if _, ok := event.Metadata["gray_release"]; ok {
		t.Fatal("gray routing mutated caller-owned Metadata")
	}
}
