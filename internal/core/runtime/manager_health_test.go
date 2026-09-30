package runtime

import (
	"fmt"
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

func TestManagerApplyNodeHealthUpdateTracksStateAndHeartbeat(t *testing.T) {
	mgr := NewManager()
	inst, err := mgr.CreateRuleInstance("rule-health", "1")
	if err != nil {
		t.Fatalf("CreateRuleInstance failed: %v", err)
	}

	nodeID := inst.ID + "-node-1"
	if err := mgr.AddNodeInstance(inst.ID, &NodeInstance{
		ID:        nodeID,
		NodeID:    "node-1",
		PluginID:  "source-mc",
		Status:    NodeStatusInit,
		StartedAt: time.Now(),
		Health:    NodeHealth{Status: sdk.HealthUnknown},
	}); err != nil {
		t.Fatalf("AddNodeInstance failed: %v", err)
	}

	degradedAt := time.Now().Add(-2 * time.Second).Truncate(time.Microsecond)
	if err := mgr.ApplyNodeHealthUpdate(inst.ID, nodeID, sdk.ReportHealthArgs{
		Kind:               sdk.HealthKindState,
		Status:             sdk.HealthDegraded,
		Code:               "NET_PLC_UNREACHABLE",
		Message:            "plc unreachable",
		Details:            map[string]string{"target": "192.0.2.10"},
		ObservedAtUnixNano: degradedAt.UnixNano(),
		Sequence:           7,
	}); err != nil {
		t.Fatalf("ApplyNodeHealthUpdate degraded failed: %v", err)
	}

	instance, err := mgr.GetRuleInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetRuleInstance failed: %v", err)
	}
	if instance.Nodes[nodeID].Health.Status != sdk.HealthDegraded {
		t.Fatalf("expected degraded status, got %s", instance.Nodes[nodeID].Health.Status)
	}
	if instance.Nodes[nodeID].Status != NodeStatusError {
		t.Fatalf("expected node status error after degraded health, got %s", instance.Nodes[nodeID].Status)
	}
	if instance.Nodes[nodeID].Health.Code != "NET_PLC_UNREACHABLE" {
		t.Fatalf("expected degraded code, got %s", instance.Nodes[nodeID].Health.Code)
	}
	if instance.Nodes[nodeID].LastError == nil || instance.Nodes[nodeID].LastError.Message != "plc unreachable" {
		t.Fatalf("expected last error to be recorded, got %#v", instance.Nodes[nodeID].LastError)
	}
	if len(instance.Nodes[nodeID].ErrorHistory) != 1 || instance.Nodes[nodeID].ErrorHistory[0].Message != "plc unreachable" {
		t.Fatalf("expected error history to contain degraded error, got %#v", instance.Nodes[nodeID].ErrorHistory)
	}
	if instance.Nodes[nodeID].Health.Sequence != 7 {
		t.Fatalf("expected sequence 7, got %d", instance.Nodes[nodeID].Health.Sequence)
	}
	if instance.Nodes[nodeID].Health.LastHealthyAt != nil {
		t.Fatal("expected LastHealthyAt to be nil before healthy report")
	}

	healthyAt := degradedAt.Add(time.Second)
	if err := mgr.ApplyNodeHealthUpdate(inst.ID, nodeID, sdk.ReportHealthArgs{
		Kind:               sdk.HealthKindState,
		Status:             sdk.HealthHealthy,
		Message:            "plc recovered",
		ObservedAtUnixNano: healthyAt.UnixNano(),
		Sequence:           8,
	}); err != nil {
		t.Fatalf("ApplyNodeHealthUpdate healthy failed: %v", err)
	}

	instance, err = mgr.GetRuleInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetRuleInstance failed after healthy update: %v", err)
	}
	if instance.Nodes[nodeID].Health.Status != sdk.HealthHealthy {
		t.Fatalf("expected healthy status, got %s", instance.Nodes[nodeID].Health.Status)
	}
	if instance.Nodes[nodeID].Status != NodeStatusRunning {
		t.Fatalf("expected node status running after healthy health, got %s", instance.Nodes[nodeID].Status)
	}
	if instance.Nodes[nodeID].Health.LastHealthyAt == nil || !instance.Nodes[nodeID].Health.LastHealthyAt.Equal(healthyAt) {
		t.Fatalf("expected LastHealthyAt=%v, got %+v", healthyAt, instance.Nodes[nodeID].Health.LastHealthyAt)
	}

	heartbeatAt := healthyAt.Add(time.Second)
	if err := mgr.ApplyNodeHealthUpdate(inst.ID, nodeID, sdk.ReportHealthArgs{
		Kind:               sdk.HealthKindHeartbeat,
		Message:            "poll loop alive",
		ObservedAtUnixNano: heartbeatAt.UnixNano(),
		Sequence:           9,
	}); err != nil {
		t.Fatalf("ApplyNodeHealthUpdate heartbeat failed: %v", err)
	}

	instance, err = mgr.GetRuleInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetRuleInstance failed after heartbeat: %v", err)
	}
	if instance.Nodes[nodeID].Health.Status != sdk.HealthHealthy {
		t.Fatalf("heartbeat should not change status, got %s", instance.Nodes[nodeID].Health.Status)
	}
	if instance.Nodes[nodeID].Status != NodeStatusRunning {
		t.Fatalf("heartbeat should keep node running, got %s", instance.Nodes[nodeID].Status)
	}
	if instance.Nodes[nodeID].Health.LastHeartbeatAt == nil || !instance.Nodes[nodeID].Health.LastHeartbeatAt.Equal(heartbeatAt) {
		t.Fatalf("expected LastHeartbeatAt=%v, got %+v", heartbeatAt, instance.Nodes[nodeID].Health.LastHeartbeatAt)
	}
}

func TestManagerApplyNodeHealthUpdateIgnoresStaleReports(t *testing.T) {
	mgr := NewManager()
	inst, err := mgr.CreateRuleInstance("rule-health", "1")
	if err != nil {
		t.Fatalf("CreateRuleInstance failed: %v", err)
	}

	nodeID := inst.ID + "-node-2"
	if err := mgr.AddNodeInstance(inst.ID, &NodeInstance{
		ID:        nodeID,
		NodeID:    "node-2",
		PluginID:  "sink-mc",
		Status:    NodeStatusInit,
		StartedAt: time.Now(),
		Health:    NodeHealth{Status: sdk.HealthUnknown},
	}); err != nil {
		t.Fatalf("AddNodeInstance failed: %v", err)
	}

	latestAt := time.Now().Truncate(time.Microsecond)
	if err := mgr.ApplyNodeHealthUpdate(inst.ID, nodeID, sdk.ReportHealthArgs{
		Kind:               sdk.HealthKindState,
		Status:             sdk.HealthHealthy,
		Message:            "connected",
		ObservedAtUnixNano: latestAt.UnixNano(),
		Sequence:           5,
	}); err != nil {
		t.Fatalf("latest health update failed: %v", err)
	}

	staleAt := latestAt.Add(-time.Second)
	if err := mgr.ApplyNodeHealthUpdate(inst.ID, nodeID, sdk.ReportHealthArgs{
		Kind:               sdk.HealthKindState,
		Status:             sdk.HealthUnhealthy,
		Code:               "NET_WRITE_TIMEOUT",
		Message:            "stale timeout",
		ObservedAtUnixNano: staleAt.UnixNano(),
		Sequence:           4,
	}); err != nil {
		t.Fatalf("stale health update failed: %v", err)
	}

	instance, err := mgr.GetRuleInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetRuleInstance failed: %v", err)
	}
	if instance.Nodes[nodeID].Health.Status != sdk.HealthHealthy {
		t.Fatalf("expected stale update to be ignored, got %s", instance.Nodes[nodeID].Health.Status)
	}
	if instance.Nodes[nodeID].Health.Code != "" {
		t.Fatalf("expected stale code to be ignored, got %s", instance.Nodes[nodeID].Health.Code)
	}
}

func TestManagerRecordNodeErrorKeepsLatestHistoryWithinLimit(t *testing.T) {
	mgr := NewManagerWithOptions(ManagerOptions{MaxNodeErrorHistory: 3})
	inst, err := mgr.CreateRuleInstance("rule-errors", "1")
	if err != nil {
		t.Fatalf("CreateRuleInstance failed: %v", err)
	}

	nodeID := inst.ID + "-node-3"
	if err := mgr.AddNodeInstance(inst.ID, &NodeInstance{
		ID:        nodeID,
		NodeID:    "node-3",
		PluginID:  "source-s7",
		Status:    NodeStatusInit,
		StartedAt: time.Now(),
		Health:    NodeHealth{Status: sdk.HealthUnknown},
	}); err != nil {
		t.Fatalf("AddNodeInstance failed: %v", err)
	}

	for i := 1; i <= 5; i++ {
		if err := mgr.RecordNodeError(inst.ID, nodeID, "", fmt.Sprintf("error-%d", i)); err != nil {
			t.Fatalf("RecordNodeError failed: %v", err)
		}
	}

	instance, err := mgr.GetRuleInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetRuleInstance failed: %v", err)
	}

	history := instance.Nodes[nodeID].ErrorHistory
	if len(history) != 3 {
		t.Fatalf("expected 3 history errors, got %d", len(history))
	}
	if history[0].Message != "error-3" || history[1].Message != "error-4" || history[2].Message != "error-5" {
		t.Fatalf("expected latest 3 errors to be kept, got %#v", history)
	}
	if instance.Nodes[nodeID].LastError == nil || instance.Nodes[nodeID].LastError.Message != "error-5" {
		t.Fatalf("expected last error to track most recent error, got %#v", instance.Nodes[nodeID].LastError)
	}
}
