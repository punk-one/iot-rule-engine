package engine

import (
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestRuleInstanceBuilder_CheckAndStopExistingInstance(t *testing.T) {
	// 测试当没有已存在实例时的行为
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	// 设置一个模拟的 RuntimeManager
	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr

	dag := &DAG{
		RuleID: "test-rule-1",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-1",
			Version: 1,
			Enabled: true,
		},
	}

	builder := newRuleInstanceBuilder(executor, dag)

	// 没有已存在的实例，应该返回 shouldSkip=false
	shouldSkip, err := builder.checkAndStopExistingInstance()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if shouldSkip {
		t.Error("Should not skip when no existing instance")
	}
}

func TestRuleInstanceBuilder_CreateRuleInstance(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr

	dag := &DAG{
		RuleID: "test-rule-2",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-2",
			Version: 1,
			Enabled: true,
		},
	}

	builder := newRuleInstanceBuilder(executor, dag)

	instance, err := builder.createRuleInstance()
	if err != nil {
		t.Errorf("Failed to create rule instance: %v", err)
	}
	if instance == nil {
		t.Error("Instance should not be nil")
	}
	if instance.RuleID != "test-rule-2" {
		t.Errorf("Expected RuleID 'test-rule-2', got '%s'", instance.RuleID)
	}
}

func TestRuleInstanceBuilder_CheckAndStopExistingInstance_DeletesOldInstance(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr

	// 创建并启动旧版本实例（rule version = 1）
	oldInstance, err := runtimeMgr.CreateRuleInstance("test-rule-delete-old", "1")
	if err != nil {
		t.Fatalf("Failed to create old instance: %v", err)
	}
	if err := runtimeMgr.StartRuleInstance(oldInstance.ID); err != nil {
		t.Fatalf("Failed to start old instance: %v", err)
	}

	// 新配置版本更高（version = 2），应触发旧实例删除
	dag := &DAG{
		RuleID: "test-rule-delete-old",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-delete-old",
			Version: 2,
			Enabled: true,
		},
	}
	builder := newRuleInstanceBuilder(executor, dag)

	shouldSkip, err := builder.checkAndStopExistingInstance()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if shouldSkip {
		t.Fatal("Should not skip when version changes")
	}

	if _, err := runtimeMgr.GetRuleInstance(oldInstance.ID); err == nil {
		t.Fatalf("Old instance %s should be deleted after version switch", oldInstance.ID)
	}
}

func TestRuleInstanceBuilder_InitializeNodeInstances(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr

	dag := &DAG{
		RuleID: "test-rule-3",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-3",
			Version: 1,
			Enabled: true,
			Nodes: []*schema.NodeConfig{
				{ID: "source-1", Type: "source"},
				{ID: "processor-1", Type: "processor", Wires: []string{}},
				{ID: "sink-1", Type: "sink"},
			},
		},
	}

	builder := newRuleInstanceBuilder(executor, dag)

	// 先创建 RuleInstance
	instance, err := builder.createRuleInstance()
	if err != nil {
		t.Fatalf("Failed to create rule instance: %v", err)
	}

	// 初始化 NodeInstances
	err = builder.initializeNodeInstances(instance)
	if err != nil {
		t.Errorf("Failed to initialize node instances: %v", err)
	}

	// 验证节点数量：从 instance.Nodes 获取
	if len(instance.Nodes) != 3 {
		t.Errorf("Expected 3 node instances, got %d", len(instance.Nodes))
	}
}

func TestRuleInstanceBuilder_InitializeEdgeInstances(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	runtimeMgr := runtime.NewManager()
	executor.runtimeMgr = runtimeMgr

	dag := &DAG{
		RuleID: "test-rule-4",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-4",
			Version: 1,
			Enabled: true,
			Nodes: []*schema.NodeConfig{
				{ID: "source-1", Type: "source", Wires: []string{"processor-1"}},
				{ID: "processor-1", Type: "processor", Wires: []string{"sink-1"}},
				{ID: "sink-1", Type: "sink", Wires: []string{}},
			},
		},
	}

	builder := newRuleInstanceBuilder(executor, dag)

	// 创建 RuleInstance
	instance, err := builder.createRuleInstance()
	if err != nil {
		t.Fatalf("Failed to create rule instance: %v", err)
	}

	// 初始化 EdgeInstances
	err = builder.initializeEdgeInstances(instance)
	if err != nil {
		t.Errorf("Failed to initialize edge instances: %v", err)
	}

	// 验证边数量 (source->processor, processor->sink = 2 edges)
	if len(instance.Edges) != 2 {
		t.Errorf("Expected 2 edge instances, got %d", len(instance.Edges))
	}
}

func TestComputeNodeEdges(t *testing.T) {
	executor, err := NewExecutor()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}
	defer executor.Close()

	dag := &DAG{
		RuleID: "test-rule-5",
		RuleFlow: &schema.RuleFlow{
			ID:      "test-rule-5",
			Version: 1,
			Nodes: []*schema.NodeConfig{
				{ID: "source-1", Type: "source", Wires: []string{"processor-1"}},
				{ID: "processor-1", Type: "processor", Wires: []string{"sink-1"}},
				{ID: "sink-1", Type: "sink", Wires: []string{}},
			},
		},
	}

	builder := newRuleInstanceBuilder(executor, dag)

	// 测试 processor-1 的边计算
	nodeInstance := &runtime.NodeInstance{
		ID:          "test-processor-1",
		NodeID:      "processor-1",
		InputEdges:  []string{},
		OutputEdges: []string{},
	}

	builder.computeNodeEdges(nodeInstance, dag.RuleFlow.Nodes[1])

	// processor-1 应该有 1 个输入边 (从 source-1) 和 1 个输出边 (到 sink-1)
	if len(nodeInstance.InputEdges) != 1 {
		t.Errorf("Expected 1 input edge, got %d", len(nodeInstance.InputEdges))
	}
	if len(nodeInstance.OutputEdges) != 1 {
		t.Errorf("Expected 1 output edge, got %d", len(nodeInstance.OutputEdges))
	}
}
