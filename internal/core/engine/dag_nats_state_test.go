package engine

import (
	"testing"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/pkg/state"
)

func TestCreateRuntimeContextProvidesStateManager(t *testing.T) {
	executor := &DAGNATSExecutor{
		dag: &DAG{
			RuleID: "rule-test",
			RuleFlow: &schema.RuleFlow{
				ID: "rule-test",
				Nodes: []*schema.NodeConfig{
					{ID: "detect-node"},
				},
			},
		},
		stateMgr: state.NewStateManager(state.NewMemoryBackend()),
	}

	ctx := executor.CreateRuntimeContext("detect-node")

	statefulCtx, ok := ctx.(sdk.StatefulRuntimeContext)
	if !ok {
		t.Fatalf("expected runtime context to implement sdk.StatefulRuntimeContext")
	}
	if statefulCtx.State() == nil {
		t.Fatalf("expected state manager to be available")
	}

	type snapshot struct {
		Value int `json:"value"`
	}

	key := statefulCtx.State().GenerateKey([]string{"acm006"})
	if err := statefulCtx.State().SetState(key, &snapshot{Value: 7}); err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	var got snapshot
	if err := statefulCtx.State().GetState(key, &got); err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if got.Value != 7 {
		t.Fatalf("expected state value 7, got %d", got.Value)
	}
}

func TestCreateRuntimeContextStateManagerSupportsTTL(t *testing.T) {
	executor := &DAGNATSExecutor{
		dag: &DAG{
			RuleID: "rule-test",
			RuleFlow: &schema.RuleFlow{
				ID: "rule-test",
				Nodes: []*schema.NodeConfig{
					{ID: "detect-node"},
				},
			},
		},
		stateMgr: state.NewStateManager(state.NewMemoryBackend()),
	}

	ctx := executor.CreateRuntimeContext("detect-node")
	statefulCtx, ok := ctx.(sdk.StatefulRuntimeContext)
	if !ok {
		t.Fatalf("expected runtime context to implement sdk.StatefulRuntimeContext")
	}

	type snapshot struct {
		Value int `json:"value"`
	}

	key := statefulCtx.State().GenerateKey([]string{"ttl"})
	if err := statefulCtx.State().SetStateWithTTL(key, &snapshot{Value: 5}, 20*time.Millisecond); err != nil {
		t.Fatalf("SetStateWithTTL failed: %v", err)
	}

	var got snapshot
	if err := statefulCtx.State().GetState(key, &got); err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if got.Value != 5 {
		t.Fatalf("expected 5, got %d", got.Value)
	}

	time.Sleep(30 * time.Millisecond)

	if err := statefulCtx.State().GetState(key, &got); err == nil {
		t.Fatal("expected state to expire")
	}
}
