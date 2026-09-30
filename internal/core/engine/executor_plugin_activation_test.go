package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	pluginruntime "github.com/punk-one/iot-rule-engine/internal/core/plugin/instance"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
)

type fakeInstanceRegistry struct {
	instances map[string]*pluginruntime.PluginInstance
	startErr  error
}

func (r *fakeInstanceRegistry) Register(inst *pluginruntime.PluginInstance) error {
	if r.instances == nil {
		r.instances = make(map[string]*pluginruntime.PluginInstance)
	}
	r.instances[inst.ID] = inst
	return nil
}

func (r *fakeInstanceRegistry) Start(id string) error {
	if r.startErr != nil {
		return r.startErr
	}
	return nil
}

func (r *fakeInstanceRegistry) Stop(id string) error   { return nil }
func (r *fakeInstanceRegistry) Remove(id string) error { delete(r.instances, id); return nil }
func (r *fakeInstanceRegistry) Get(id string) (*pluginruntime.PluginInstance, bool) {
	inst, ok := r.instances[id]
	return inst, ok
}
func (r *fakeInstanceRegistry) ListByRule(ruleID string) []*pluginruntime.PluginInstance { return nil }
func (r *fakeInstanceRegistry) ListAll() []*pluginruntime.PluginInstance                 { return nil }

func TestStartPluginInstancesForRule_FailsFastOnPluginStartError(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr
	executor.pluginRegistry = &fakeInstanceRegistry{
		startErr: fmt.Errorf("state manager not available, ChangeDetect requires stateful support"),
	}

	dag := &DAG{
		RuleID: "rule-detect-change",
		RuleFlow: &schema.RuleFlow{
			ID:      "rule-detect-change",
			Version: 1,
			Enabled: true,
			Nodes: []*schema.NodeConfig{
				{
					ID:     "processor_1774420244857",
					Type:   "processor",
					Plugin: "detect-change",
				},
			},
		},
	}

	instance, err := runtimeMgr.CreateRuleInstance(dag.RuleID, "1")
	if err != nil {
		t.Fatalf("Failed to create rule instance: %v", err)
	}
	nodeInstance := &runtime.NodeInstance{
		ID:       fmt.Sprintf("%s-%s", instance.ID, "processor_1774420244857"),
		NodeID:   "processor_1774420244857",
		PluginID: "detect-change",
		Status:   runtime.NodeStatusInit,
	}
	if err := runtimeMgr.AddNodeInstance(instance.ID, nodeInstance); err != nil {
		t.Fatalf("Failed to add node instance: %v", err)
	}

	err = executor.startPluginInstancesForRule(dag, &DAGNATSExecutor{dag: dag}, instance)
	if err == nil {
		t.Fatal("Expected plugin activation to fail")
	}
	if !strings.Contains(err.Error(), "processor_1774420244857") {
		t.Fatalf("Expected error to mention failing node, got: %v", err)
	}

	stored, getErr := runtimeMgr.GetRuleInstance(instance.ID)
	if getErr != nil {
		t.Fatalf("Failed to reload rule instance: %v", getErr)
	}
	node := stored.Nodes[nodeInstance.ID]
	if node == nil {
		t.Fatalf("Expected node instance %s to exist", nodeInstance.ID)
	}
	if node.Status != runtime.NodeStatusError {
		t.Fatalf("Expected node status error, got %s", node.Status)
	}
	if node.LastError == nil || !strings.Contains(node.LastError.Message, "state manager not available") {
		t.Fatalf("Expected last error to be recorded, got %#v", node.LastError)
	}
}
