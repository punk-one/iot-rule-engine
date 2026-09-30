package engine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// ruleInstanceBuilder 规则实例构建器
// 将 startRuleInstance 的逻辑拆分为更小、更易测试的方法
type ruleInstanceBuilder struct {
	dag        *DAG
	runtimeMgr *runtime.Manager
	stateMgr   interface{} // *state.StateManager
	executor   *Executor
}

// newRuleInstanceBuilder 创建规则实例构建器
func newRuleInstanceBuilder(e *Executor, dag *DAG) *ruleInstanceBuilder {
	return &ruleInstanceBuilder{
		dag:        dag,
		runtimeMgr: e.runtimeMgr,
		stateMgr:   e.stateMgr,
		executor:   e,
	}
}

// checkAndStopExistingInstance 检查并停止已存在的规则实例
// 如果存在相同版本的运行实例，返回 nil（跳过启动）
// 如果存在不同版本的运行实例，停止旧实例并返回 nil（继续启动）
func (b *ruleInstanceBuilder) checkAndStopExistingInstance() (shouldSkip bool, err error) {
	existingInstance, err := b.runtimeMgr.GetRuleInstanceByRuleID(b.dag.RuleID)
	if err != nil || existingInstance == nil {
		return false, nil // 没有已存在的实例，继续启动
	}

	if existingInstance.Status != runtime.RuleStatusRunning {
		return false, nil // 实例不在运行中，继续启动
	}

	// 检查版本号
	existingVersion, _ := strconv.Atoi(existingInstance.Version)
	if existingVersion == b.dag.RuleFlow.Version {
		pkglog.Debug("Rule instance already running for rule %s with version %d", b.dag.RuleID, b.dag.RuleFlow.Version)
		return true, nil // 相同版本已在运行，跳过启动
	}

	// 版本不同，需要停止旧实例
	pkglog.Info("Stopping old version instance %s (version %s) for rule %s, starting new version %d",
		existingInstance.ID, existingInstance.Version, b.dag.RuleID, b.dag.RuleFlow.Version)
	if err := b.runtimeMgr.StopRuleInstance(existingInstance.ID); err != nil {
		pkglog.Error("Failed to stop old instance %s: %v", existingInstance.ID, err)
	}
	// 清理旧实例，避免 runtime 列表残留历史实例。
	if err := b.runtimeMgr.DeleteRuleInstance(existingInstance.ID); err != nil {
		pkglog.Error("Failed to delete old instance %s: %v", existingInstance.ID, err)
	}

	return false, nil
}

// createRuleInstance 创建 RuleInstance
func (b *ruleInstanceBuilder) createRuleInstance() (*runtime.RuleInstance, error) {
	version := strconv.Itoa(b.dag.RuleFlow.Version)
	instance, err := b.runtimeMgr.CreateRuleInstance(b.dag.RuleID, version)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule instance: %w", err)
	}
	return instance, nil
}

// initializeNodeInstances 初始化所有 NodeInstance
func (b *ruleInstanceBuilder) initializeNodeInstances(instance *runtime.RuleInstance) error {
	version := strconv.Itoa(b.dag.RuleFlow.Version)

	for _, nodeConfig := range b.dag.RuleFlow.Nodes {
		nodeInstance := &runtime.NodeInstance{
			ID:          fmt.Sprintf("%s-%s", instance.ID, nodeConfig.ID),
			NodeID:      nodeConfig.ID,
			PluginID:    nodeConfig.Type,
			Version:     version,
			Config:      cloneNodeRuntimeConfig(nodeConfig.Config),
			Status:      runtime.NodeStatusInit,
			StartedAt:   time.Now(),
			InputEdges:  []string{},
			OutputEdges: []string{},
			Metrics:     runtime.NodeMetrics{},
			Health:      runtime.NodeHealth{Status: sdk.HealthUnknown},
		}

		// 计算输入边和输出边
		b.computeNodeEdges(nodeInstance, nodeConfig)

		if err := b.runtimeMgr.AddNodeInstance(instance.ID, nodeInstance); err != nil {
			pkglog.Error("Failed to add node instance %s: %v", nodeInstance.ID, err)
			continue
		}
	}

	return nil
}

// computeNodeEdges 计算节点的输入边和输出边
func (b *ruleInstanceBuilder) computeNodeEdges(nodeInstance *runtime.NodeInstance, nodeConfig interface{}) {
	for _, edge := range collectTopologyEdges(b.dag.RuleFlow) {
		edgeID := topologyEdgeID(edge)
		if edge.To == nodeInstance.NodeID {
			nodeInstance.InputEdges = append(nodeInstance.InputEdges, edgeID)
		}
		if edge.From == nodeInstance.NodeID {
			nodeInstance.OutputEdges = append(nodeInstance.OutputEdges, edgeID)
		}
	}
}

// initializeEdgeInstances 初始化所有 EdgeInstance
func (b *ruleInstanceBuilder) initializeEdgeInstances(instance *runtime.RuleInstance) error {
	for _, edge := range collectTopologyEdges(b.dag.RuleFlow) {
		edgeID := topologyEdgeID(edge)
		edgeInstance := &runtime.EdgeInstance{
			ID:       fmt.Sprintf("%s-%s", instance.ID, edgeID),
			EdgeID:   edgeID,
			FromNode: edge.From,
			ToNode:   edge.To,
			Topic:    topologyEdgeTopic(b.dag.RuleID, edge),
			Metrics:  runtime.EdgeMetrics{},
		}

		if err := b.runtimeMgr.AddEdgeInstance(instance.ID, edgeInstance); err != nil {
			pkglog.Error("Failed to add edge instance %s: %v", edgeInstance.ID, err)
			continue
		}
	}

	return nil
}

// createAndStartExecutor 创建并启动 DAGNATSExecutor
func (b *ruleInstanceBuilder) createAndStartExecutor(instance *runtime.RuleInstance) (*DAGNATSExecutor, error) {
	var executor *DAGNATSExecutor
	if b.stateMgr != nil {
		// 需要类型断言
		if sm, ok := b.stateMgr.(interface {
			GetBackend() interface{}
		}); ok {
			_ = sm // 使用 state manager
		}
		executor = NewDAGNATSExecutor(b.dag, b.runtimeMgr, instance.ID)
	} else {
		executor = NewDAGNATSExecutor(b.dag, b.runtimeMgr, instance.ID)
	}

	// 存储 DAGNATSExecutor
	b.executor.mutex.Lock()
	b.executor.executors[b.dag.RuleID] = executor
	b.executor.mutex.Unlock()

	ctx := context.Background()
	if err := executor.Start(ctx); err != nil {
		// Cleanup
		b.executor.mutex.Lock()
		delete(b.executor.executors, b.dag.RuleID)
		b.executor.mutex.Unlock()
		return nil, fmt.Errorf("failed to start DAGNATSExecutor: %w", err)
	}

	return executor, nil
}

// loadAndStartPlugins 加载并启动插件实例
func (b *ruleInstanceBuilder) loadAndStartPlugins(executor *DAGNATSExecutor, instance *runtime.RuleInstance) error {
	if b.executor.pluginRegistry == nil {
		return nil
	}

	pluginInstances := b.executor.buildPluginInstances(b.dag.RuleFlow)

	for _, inst := range pluginInstances {
		// Register
		if err := b.executor.pluginRegistry.Register(inst); err != nil {
			pkglog.Error("Failed to register plugin instance %s: %v", inst.ID, err)
			continue
		}

		// Start
		if err := b.executor.pluginRegistry.Start(inst.ID); err != nil {
			pkglog.Error("Failed to start plugin instance %s: %v", inst.ID, err)
			continue
		}

		// Register to Dispatcher
		if pluginClient, ok := inst.Internal.(*plugin.PluginClient); ok {
			if err := executor.RegisterPlugin(inst.NodeID, pluginClient.RPCClient); err != nil {
				pkglog.Error("Failed to register plugin %s to dispatcher: %v", inst.NodeID, err)
			} else {
				pkglog.Debug("Registered plugin %s to dispatcher (rule: %s)", inst.NodeID, b.dag.RuleID)
			}
		} else {
			pkglog.Warn("Plugin instance %s has no internal client", inst.ID)
		}

		// Update Node Status
		nodeInstanceID := fmt.Sprintf("%s-%s", instance.ID, inst.NodeID)
		_ = b.runtimeMgr.UpdateNodeStatus(instance.ID, nodeInstanceID, runtime.NodeStatusRunning)
	}

	return nil
}

// Note: BuildPluginInstances is defined in instance_builder.go
