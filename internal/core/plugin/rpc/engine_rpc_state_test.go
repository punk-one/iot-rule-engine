package rpc

import (
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-engine/pkg/state"
)

type allowRegistry struct{}

func (allowRegistry) IsPluginRegistered(ruleID, nodeID string) bool { return true }

func TestEngineRPCServerStateRoundTrip(t *testing.T) {
	stateMgr := state.NewStateManager(state.NewMemoryBackend())
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		stateMgr,
		allowRegistry{},
		nil,
		nil,
		nil,
	)

	payload := []byte(`{"value":42}`)
	if err := server.SetState("state:test", payload); err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	got, err := server.GetState("state:test")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("expected %s, got %s", payload, got)
	}

	if err := server.DeleteState("state:test"); err != nil {
		t.Fatalf("DeleteState failed: %v", err)
	}

	got, err = server.GetState("state:test")
	if err != nil {
		t.Fatalf("GetState after delete failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty state after delete, got %q", got)
	}
}

func TestEngineRPCServerStateTTLRoundTrip(t *testing.T) {
	stateMgr := state.NewStateManager(state.NewMemoryBackend())
	server := NewEngineRPCServerWithRegistry(
		nil,
		&sdk.DefaultLogger{},
		"rule-test",
		"node-test",
		nil,
		nil,
		stateMgr,
		allowRegistry{},
		nil,
		nil,
		nil,
	)

	payload := []byte(`{"value":42}`)
	if err := server.SetStateWithTTL("state:ttl", payload, 20*time.Millisecond); err != nil {
		t.Fatalf("SetStateWithTTL failed: %v", err)
	}

	got, err := server.GetState("state:ttl")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("expected %s, got %s", payload, got)
	}

	time.Sleep(30 * time.Millisecond)

	got, err = server.GetState("state:ttl")
	if err != nil {
		t.Fatalf("GetState after ttl failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty state after ttl, got %q", got)
	}
}
