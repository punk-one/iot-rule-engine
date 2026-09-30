package rpc

import (
	"testing"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type healthSinkStub struct {
	updates []sdk.ReportHealthArgs
}

func (s *healthSinkStub) ApplyHealthUpdate(update sdk.ReportHealthArgs) error {
	s.updates = append(s.updates, update)
	return nil
}

type denyRegistry struct{}

func (denyRegistry) IsPluginRegistered(ruleID, nodeID string) bool { return false }

func TestEngineRPCServerReportHealth(t *testing.T) {
	sink := &healthSinkStub{}
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		nil,
		allowRegistry{},
		sink,
		nil,
		nil,
	)

	update := sdk.ReportHealthArgs{
		Kind:     sdk.HealthKindState,
		Status:   sdk.HealthRecovering,
		Code:     "NET_RECONNECTING",
		Message:  "reconnecting",
		Sequence: 3,
	}
	if err := server.ReportHealth(update); err != nil {
		t.Fatalf("ReportHealth failed: %v", err)
	}
	if len(sink.updates) != 1 {
		t.Fatalf("expected 1 health update, got %d", len(sink.updates))
	}
	if sink.updates[0].Code != "NET_RECONNECTING" {
		t.Fatalf("expected forwarded code, got %s", sink.updates[0].Code)
	}
}

func TestEngineRPCServerReportHealthUnauthorized(t *testing.T) {
	sink := &healthSinkStub{}
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		nil,
		denyRegistry{},
		sink,
		nil,
		nil,
	)

	if err := server.ReportHealth(sdk.ReportHealthArgs{Kind: sdk.HealthKindHeartbeat}); err == nil {
		t.Fatal("expected unauthorized health report to fail")
	}
	if len(sink.updates) != 0 {
		t.Fatalf("expected no forwarded updates on unauthorized report, got %d", len(sink.updates))
	}
}
