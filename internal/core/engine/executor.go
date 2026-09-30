package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
	connectorruntime "github.com/punk-one/iot-rule-engine/internal/core/connector"
	"github.com/punk-one/iot-rule-engine/internal/core/dlq"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	pluginruntime "github.com/punk-one/iot-rule-engine/internal/core/plugin/instance"
	"github.com/punk-one/iot-rule-engine/internal/core/rulesanitize"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/state"
	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// Executor 负责执行规则
type Executor struct {
	dags           map[string]*DAG             // RuleID -> DAG
	executors      map[string]*DAGNATSExecutor // RuleID -> DAGNATSExecutor
	mutex          sync.RWMutex
	ruleUpdateMu   sync.Map            // ruleID -> *sync.Mutex, serializes per-rule update/start cycles
	pool           *ants.Pool          // Goroutine 池
	messageCount   int64               // 消息计数（atomic）
	errorCount     int64               // 错误计数（atomic）
	defaultTimeout time.Duration       // 默认执行超时时间
	pluginLoader   *plugin.Loader      // 插件加载器
	runtimeMgr     *runtime.Manager    // Runtime Manager
	stateMgr       *state.StateManager // State Manager
	dispatcherSec  *DispatcherSecurityConfig
	pluginDefaults PluginRuntimeDefaults
	resourceBus    *connectorruntime.ResourceBus
	connectorHost  *connectorruntime.Host

	// Plugin Governance
	pluginRegistry pluginruntime.InstanceRegistry
	authFactory    pluginruntime.ContextFactory
}

// NewExecutor 创建新的 Executor
func NewExecutor() (*Executor, error) {
	return NewExecutorWithLoader(nil, nil)
}

// NewExecutorWithLoader 创建新的 Executor（带插件加载器）
func NewExecutorWithLoader(pluginLoader *plugin.Loader, runtimeMgr *runtime.Manager) (*Executor, error) {
	// 创建 Goroutine 池（最大 1000 个并发）
	pool, err := ants.NewPool(1000, ants.WithNonblocking(false))
	if err != nil {
		return nil, fmt.Errorf("failed to create goroutine pool: %w", err)
	}

	e := &Executor{
		dags:           make(map[string]*DAG),
		executors:      make(map[string]*DAGNATSExecutor),
		pool:           pool,
		defaultTimeout: 30 * time.Second,
		pluginLoader:   pluginLoader,
		runtimeMgr:     runtimeMgr,
	}

	// Initialize Plugin Runtime Governance
	if pluginLoader != nil {
		e.resourceBus = connectorruntime.NewResourceBus(connectorruntime.ResourceBusOptions{})
		e.connectorHost = connectorruntime.NewHost(pluginLoader)
		e.resourceBus.SetResourceProviderResolver(e.connectorHost.ResolveForResource)

		// ContextFactory: delegates to Executor to find DAGNATSExecutor and create context
		e.authFactory = func(ruleID, nodeID string) (sdk.RuntimeContext, error) {
			e.mutex.RLock()
			exec, ok := e.executors[ruleID]
			e.mutex.RUnlock()
			if !ok {
				return nil, fmt.Errorf("executor not found for rule %s", ruleID)
			}
			return exec.CreateRuntimeContext(nodeID), nil
		}

		runner := pluginruntime.NewGoPluginRunner(pluginLoader, e.authFactory)
		e.pluginRegistry = pluginruntime.NewMemoryRegistry(runner)
	}

	return e, nil
}

// Close 关闭 Executor（释放资源）
func (e *Executor) Close() {
	if e.pool != nil {
		e.pool.Release()
	}
	if e.connectorHost != nil {
		e.connectorHost.Close()
	}
}

// RegisterConnectorResource 注册或更新一个连接器资源到共享 ResourceBus。
func (e *Executor) RegisterConnectorResource(resource sdk.ConnectorResource) error {
	if e.resourceBus == nil {
		return fmt.Errorf("connector runtime is not initialized")
	}
	if err := e.resourceBus.RegisterResource(resource); err != nil {
		return err
	}

	switch resource.WarmupPolicy {
	case sdk.WarmupPolicyPreload, sdk.WarmupPolicyAlwaysOn:
		if resource.Enabled {
			if err := e.resourceBus.Warmup(context.Background(), resource.ID); err != nil {
				return fmt.Errorf("failed to warm up connector resource %s: %w", resource.ID, err)
			}
		}
	}

	return nil
}

// DeleteConnectorResource 从共享 ResourceBus 中删除一个连接器资源。
func (e *Executor) DeleteConnectorResource(resourceID string) error {
	if e.resourceBus == nil {
		return fmt.Errorf("connector runtime is not initialized")
	}
	return e.resourceBus.UnregisterResource(context.Background(), resourceID)
}

func (e *Executor) GetConnectorResourceSnapshot(resourceID string) (connectorruntime.ResourceSnapshot, bool) {
	if e.resourceBus == nil {
		return connectorruntime.ResourceSnapshot{}, false
	}
	return e.resourceBus.Snapshot(resourceID)
}

func (e *Executor) ListConnectorResourceSnapshots() []connectorruntime.ResourceSnapshot {
	if e.resourceBus == nil {
		return nil
	}
	return e.resourceBus.ListSnapshots()
}

// RegisterDAG 注册一个 DAG
func (e *Executor) RegisterDAG(dag *DAG) error {
	if dag == nil {
		return fmt.Errorf("dag cannot be nil")
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.dags[dag.RuleID] = dag
	pkglog.Info("Registered DAG for rule %s", dag.RuleID)
	return nil
}

// startRuleInstance 启动规则实例（创建 RuleInstance 和 DAGNATSExecutor）
func (e *Executor) startRuleInstance(dag *DAG) error {
	if e.runtimeMgr == nil {
		pkglog.Warn("Runtime Manager not set, skipping rule instance creation for rule %s", dag.RuleID)
		return nil
	}

	// 检查规则是否启用
	if !dag.RuleFlow.Enabled {
		pkglog.Debug("Rule %s is disabled, skipping instance creation", dag.RuleID)
		return nil
	}

	// 检查是否已经存在运行中的实例，如果存在且版本不同，需要停止旧实例
	existingInstance, err := e.runtimeMgr.GetRuleInstanceByRuleID(dag.RuleID)
	if err == nil && existingInstance != nil && existingInstance.Status == runtime.RuleStatusRunning {
		// 检查版本号：如果版本相同，说明是同一个版本，不需要重启
		existingVersion, _ := strconv.Atoi(existingInstance.Version)
		if existingVersion == dag.RuleFlow.Version {
			pkglog.Debug("Rule instance already running for rule %s with version %d", dag.RuleID, dag.RuleFlow.Version)
			return nil
		}
		// 版本不同，需要停止旧实例
		pkglog.Info("Stopping old version instance %s (version %s) for rule %s, starting new version %d",
			existingInstance.ID, existingInstance.Version, dag.RuleID, dag.RuleFlow.Version)
		if err := e.runtimeMgr.StopRuleInstance(existingInstance.ID); err != nil {
			pkglog.Error("Failed to stop old instance %s: %v", existingInstance.ID, err)
		}
		// 清理旧实例，避免 runtime 列表残留历史实例。
		if err := e.runtimeMgr.DeleteRuleInstance(existingInstance.ID); err != nil {
			pkglog.Error("Failed to delete old instance %s: %v", existingInstance.ID, err)
		}
	}

	// 创建 RuleInstance
	version := strconv.Itoa(dag.RuleFlow.Version)
	instance, err := e.runtimeMgr.CreateRuleInstance(dag.RuleID, version)
	if err != nil {
		return fmt.Errorf("failed to create rule instance: %w", err)
	}
	instance.DebugMode = dag.RuleFlow.DebugMode

	// 初始化所有 NodeInstance
	for _, nodeConfig := range dag.RuleFlow.Nodes {
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

		// 计算输入边和输出边（同时包含 wires 和 routes）
		for _, edge := range collectTopologyEdges(dag.RuleFlow) {
			edgeID := topologyEdgeID(edge)
			if edge.To == nodeConfig.ID {
				nodeInstance.InputEdges = append(nodeInstance.InputEdges, edgeID)
			}
			if edge.From == nodeConfig.ID {
				nodeInstance.OutputEdges = append(nodeInstance.OutputEdges, edgeID)
			}
		}

		if err := e.runtimeMgr.AddNodeInstance(instance.ID, nodeInstance); err != nil {
			pkglog.Error("Failed to add node instance %s: %v", nodeInstance.ID, err)
			continue
		}
	}

	// 初始化所有 EdgeInstance
	for _, edge := range collectTopologyEdges(dag.RuleFlow) {
		edgeID := topologyEdgeID(edge)
		edgeInstance := &runtime.EdgeInstance{
			ID:       fmt.Sprintf("%s-%s", instance.ID, edgeID),
			EdgeID:   edgeID,
			FromNode: edge.From,
			ToNode:   edge.To,
			Topic:    topologyEdgeTopic(dag.RuleID, edge), // 仅用于元数据展示
			Metrics:  runtime.EdgeMetrics{},
		}

		if err := e.runtimeMgr.AddEdgeInstance(instance.ID, edgeInstance); err != nil {
			pkglog.Error("Failed to add edge instance %s: %v", edgeInstance.ID, err)
			continue
		}
	}

	// 创建并启动 DAGNATSExecutor（支持 State Backend）
	var executor *DAGNATSExecutor
	if e.stateMgr != nil {
		executor = NewDAGNATSExecutorWithDeliveryAndSecurityConfig(dag, e.runtimeMgr, instance.ID, e.stateMgr, nil, e.dispatcherSec)
	} else {
		executor = NewDAGNATSExecutorWithDeliveryAndSecurityConfig(dag, e.runtimeMgr, instance.ID, nil, nil, e.dispatcherSec)
	}
	executor.SetResourceBus(e.resourceBus)

	// 存储 DAGNATSExecutor (Must do this BEFORE starting plugins so ContextFactory works)
	// e.mutex usage? map access needs lock?
	// startRuleInstance is called from UpdateDAGFromJSON which holds lock?
	// UpdateDAGFromJSON: e.mutex.Unlock(); ... BuildDAG ... RegisterDAG ... startRuleInstance.
	// RegisterDAG holds lock.
	// startRuleInstance is private. Caller needs to populate e.executors?
	// No, startRuleInstance populates e.executors.
	// We should lock map access if needed, but executor methods are thread-safe usually?
	// The problem involves `e.executors` read in ContextFactory.
	e.mutex.Lock()
	e.executors[dag.RuleID] = executor
	e.mutex.Unlock()

	ctx := context.Background()
	// 仅启动 Dispatcher Logic (Plugin startup loop removed)
	if err := executor.Start(ctx); err != nil {
		// Cleanup
		e.mutex.Lock()
		delete(e.executors, dag.RuleID)
		e.mutex.Unlock()
		pkglog.Error("Failed to start DAGNATSExecutor for rule %s: %v", dag.RuleID, err)
		return fmt.Errorf("failed to start DAGNATSExecutor: %w", err)
	}

	// Build & Start Plugin Instances
	if err := e.startPluginInstancesForRule(dag, executor, instance); err != nil {
		pkglog.Error("Failed to activate plugins for rule %s: %v", dag.RuleID, err)
		if stopErr := executor.Stop(); stopErr != nil {
			pkglog.Error("Failed to stop DAGNATSExecutor after plugin activation failure for rule %s: %v", dag.RuleID, stopErr)
		}
		e.mutex.Lock()
		delete(e.executors, dag.RuleID)
		e.mutex.Unlock()
		if err := e.runtimeMgr.DeleteRuleInstance(instance.ID); err != nil {
			pkglog.Error("Failed to delete rule instance %s after plugin activation failure: %v", instance.ID, err)
		}
		return err
	}

	// 启动 RuleInstance
	if err := e.runtimeMgr.StartRuleInstance(instance.ID); err != nil {

		pkglog.Error("Failed to start rule instance %s: %v", instance.ID, err)
		return fmt.Errorf("failed to start rule instance: %w", err)
	}

	pkglog.Info("Started rule instance %s for rule %s", instance.ID, dag.RuleID)
	return nil
}

func (e *Executor) startPluginInstancesForRule(dag *DAG, executor *DAGNATSExecutor, instance *runtime.RuleInstance) error {
	if e.pluginRegistry == nil {
		return nil
	}

	pluginInstances := e.buildPluginInstances(dag.RuleFlow)
	cleanupIDs := make([]string, 0, len(pluginInstances))
	activationErrors := make([]string, 0)

	for _, inst := range pluginInstances {
		if err := e.pluginRegistry.Register(inst); err != nil {
			msg := fmt.Sprintf("register plugin instance for node %s: %v", inst.NodeID, err)
			pkglog.Error("Failed to register plugin instance %s: %v", inst.ID, err)
			e.recordPluginActivationFailure(instance.ID, inst.NodeID, msg)
			activationErrors = append(activationErrors, msg)
			continue
		}
		cleanupIDs = append(cleanupIDs, inst.ID)

		if err := e.pluginRegistry.Start(inst.ID); err != nil {
			msg := fmt.Sprintf("start plugin instance for node %s: %v", inst.NodeID, err)
			pkglog.Error("Failed to start plugin instance %s: %v", inst.ID, err)
			e.recordPluginActivationFailure(instance.ID, inst.NodeID, msg)
			activationErrors = append(activationErrors, msg)
			continue
		}

		pluginClient, ok := inst.Internal.(*plugin.PluginClient)
		if !ok || pluginClient == nil {
			msg := fmt.Sprintf("plugin instance for node %s has no internal client (type: %T)", inst.NodeID, inst.Internal)
			pkglog.Error("Plugin instance %s has no internal client or type mismatch (Internal type: %T)", inst.ID, inst.Internal)
			e.recordPluginActivationFailure(instance.ID, inst.NodeID, msg)
			activationErrors = append(activationErrors, msg)
			continue
		}

		if err := executor.RegisterPlugin(inst.NodeID, pluginClient.RPCClient); err != nil {
			msg := fmt.Sprintf("register plugin %s to dispatcher: %v", inst.NodeID, err)
			pkglog.Error("Failed to register plugin %s to dispatcher: %v (node: %s, rule: %s)", inst.NodeID, err, inst.NodeID, dag.RuleID)
			e.recordPluginActivationFailure(instance.ID, inst.NodeID, msg)
			activationErrors = append(activationErrors, msg)
			continue
		}

		pkglog.Debug("Registered plugin %s to dispatcher (rule: %s)", inst.NodeID, dag.RuleID)
		nodeInstanceID := fmt.Sprintf("%s-%s", instance.ID, inst.NodeID)
		_ = e.runtimeMgr.UpdateNodeStatus(instance.ID, nodeInstanceID, runtime.NodeStatusRunning)
	}

	if len(activationErrors) == 0 {
		return nil
	}

	for _, id := range cleanupIDs {
		if err := e.pluginRegistry.Stop(id); err != nil {
			pkglog.Error("Failed to stop plugin instance %s during activation rollback: %v", id, err)
		}
		if err := e.pluginRegistry.Remove(id); err != nil {
			pkglog.Error("Failed to remove plugin instance %s during activation rollback: %v", id, err)
		}
	}

	return fmt.Errorf("plugin activation failed for rule %s: %s", dag.RuleID, strings.Join(activationErrors, "; "))
}

func (e *Executor) recordPluginActivationFailure(instanceID, nodeID, message string) {
	if e.runtimeMgr == nil || instanceID == "" || nodeID == "" {
		return
	}

	runtimeNodeID := fmt.Sprintf("%s-%s", instanceID, nodeID)
	if err := e.runtimeMgr.RecordNodeError(instanceID, runtimeNodeID, "", message); err != nil {
		pkglog.Error("Failed to record node error for %s: %v", runtimeNodeID, err)
	}
}

// UnregisterDAG 取消注册一个 DAG
func (e *Executor) UnregisterDAG(ruleID string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// 停止 DAGNATSExecutor
	if executor, exists := e.executors[ruleID]; exists {
		if err := executor.Stop(); err != nil {
			pkglog.Error("Failed to stop DAGNATSExecutor for rule %s: %v", ruleID, err)
		}
		delete(e.executors, ruleID)
	}

	// Cleanup Plugin Instances via Registry
	if e.pluginRegistry != nil {
		instances := e.pluginRegistry.ListByRule(ruleID)
		for _, inst := range instances {
			if err := e.pluginRegistry.Stop(inst.ID); err != nil {
				pkglog.Error("Failed to stop plugin instance %s: %v", inst.ID, err)
			}
			if err := e.pluginRegistry.Remove(inst.ID); err != nil {
				pkglog.Error("Failed to remove plugin instance %s: %v", inst.ID, err)
			}
		}
	}

	// 清理该规则的所有 RuleInstance（包括 stopped），确保取消部署后不残留运行态数据。
	if e.runtimeMgr != nil {
		instances := e.runtimeMgr.ListRuleInstances()
		for _, instance := range instances {
			if instance.RuleID == ruleID {
				// 先停止非已停止状态实例
				if instance.Status != runtime.RuleStatusStopped {
					if err := e.runtimeMgr.StopRuleInstance(instance.ID); err != nil {
						pkglog.Error("Failed to stop rule instance %s for rule %s: %v", instance.ID, ruleID, err)
					}
				}
				// 无论当前状态如何，都删除实例（同时清理 trace/custom metrics）
				if err := e.runtimeMgr.DeleteRuleInstance(instance.ID); err != nil {
					pkglog.Error("Failed to delete rule instance %s for rule %s: %v", instance.ID, ruleID, err)
				}
			}
		}
	}

	delete(e.dags, ruleID)
	pkglog.Info("Unregistered DAG for rule %s", ruleID)
}

// GetDAG 获取 DAG
func (e *Executor) GetDAG(ruleID string) (*DAG, bool) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	dag, exists := e.dags[ruleID]
	return dag, exists
}

// Execute 执行规则消息（同步执行）
func (e *Executor) Execute(ctx context.Context, msg *schema.RuleMessage) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}

	atomic.AddInt64(&e.messageCount, 1)

	// 获取对应的 DAG
	dag, exists := e.GetDAG(msg.RuleID)
	if !exists {
		atomic.AddInt64(&e.errorCount, 1)
		return fmt.Errorf("DAG not found for rule %s", msg.RuleID)
	}

	// 版本校验：如果消息中的版本号高于本地 DAG 版本，需要重新拉取配置
	if msg.RuleID != "" {
		// 这里可以添加版本校验逻辑（需要 RuleMessage 中添加 Version 字段）
		// 暂时跳过
	}

	// 检查规则是否启用
	if !dag.RuleFlow.Enabled {
		pkglog.Debug("Rule %s is disabled, skipping execution", msg.RuleID)
		return nil
	}

	// 设置 Debug 模式
	if dag.RuleFlow.DebugMode {
		msg.IsDebug = true
	}

	select {
	case <-ctx.Done():
		atomic.AddInt64(&e.errorCount, 1)
		return ctx.Err()
	default:
	}
	e.mutex.RLock()
	running := e.executors[msg.RuleID]
	e.mutex.RUnlock()
	if running == nil {
		atomic.AddInt64(&e.errorCount, 1)
		return fmt.Errorf("rule %s is enabled but has no running executor", msg.RuleID)
	}
	if err := running.InjectRuleMessage(msg); err != nil {
		atomic.AddInt64(&e.errorCount, 1)
		return err
	}
	return nil
}

// ExecuteAsync 异步执行规则消息（使用 Goroutine 池）
func (e *Executor) ExecuteAsync(ctx context.Context, msg *schema.RuleMessage, done chan<- error) {
	if e.pool == nil {
		done <- fmt.Errorf("executor pool not initialized")
		return
	}

	err := e.pool.Submit(func() {
		err := e.Execute(ctx, msg)
		if done != nil {
			done <- err
		}
	})

	if err != nil {
		atomic.AddInt64(&e.errorCount, 1)
		if done != nil {
			done <- fmt.Errorf("failed to submit task to pool: %w", err)
		}
	}
}

// UpdateDAGFromJSON 从 JSON 更新 DAG
func (e *Executor) UpdateDAGFromJSON(jsonData []byte) error {

	var ruleFlow schema.RuleFlow

	// 尝试直接反序列化
	if err := json.Unmarshal(jsonData, &ruleFlow); err != nil {
		// 如果失败，可能是双重序列化的字符串，尝试先解析字符串
		var str string
		if err2 := json.Unmarshal(jsonData, &str); err2 == nil {
			// 成功解析为字符串，再次反序列化
			if err3 := json.Unmarshal([]byte(str), &ruleFlow); err3 != nil {
				return fmt.Errorf("failed to unmarshal RuleFlow (double-encoded string): %w", err3)
			}
		} else {
			return fmt.Errorf("failed to unmarshal RuleFlow: %w", err)
		}
	}
	rulesanitize.CleanupVirtualSourceNodes(&ruleFlow)
	unlockRule := e.lockRuleUpdate(ruleFlow.ID)
	defer unlockRule()

	// 检查是否已经存在且相同版本且状态一致
	e.mutex.Lock()
	if existing, exists := e.dags[ruleFlow.ID]; exists {
		// 如果版本号、启用状态、调试模式都一样，说明配置没变且不需要切换状态，跳过
		// 注意：RuleFlow 本身的内容可能变了（比如节点连线），所以我们这里还是简单点，只看版本
		if existing.RuleFlow.Version == ruleFlow.Version &&
			existing.RuleFlow.Enabled == ruleFlow.Enabled &&
			existing.RuleFlow.DebugMode == ruleFlow.DebugMode {

			// 如果已启用且正在运行，不需要重启
			if ruleFlow.Enabled {
				if _, err := e.runtimeMgr.GetRuleInstanceByRuleID(ruleFlow.ID); err == nil {
					e.mutex.Unlock()
					pkglog.Debug("Rule %s version %d already running, skipping update", ruleFlow.ID, ruleFlow.Version)
					return nil
				}
			} else {
				// 如果已禁用，检查是否还有运行中的实例，如果有则停止
				instance, err := e.runtimeMgr.GetRuleInstanceByRuleID(ruleFlow.ID)
				if err == nil && instance != nil && instance.Status == runtime.RuleStatusRunning {
					e.mutex.Unlock()
					pkglog.Info("Rule %s is disabled but instance is still running, stopping it", ruleFlow.ID)
					e.UnregisterDAG(ruleFlow.ID)
					return nil
				}
				e.mutex.Unlock()
				pkglog.Debug("Rule %s version %d already disabled, skipping update", ruleFlow.ID, ruleFlow.Version)
				return nil
			}
		}

		// 版本号或启用状态发生变化，需要重新注册
		e.mutex.Unlock()
		if existing.RuleFlow.Enabled != ruleFlow.Enabled {
			pkglog.Info("Rule %s enabled status changed from %v to %v, unregistering", ruleFlow.ID, existing.RuleFlow.Enabled, ruleFlow.Enabled)
		} else {
			pkglog.Info("Rule %s version or config changed (version: %d -> %d), unregistering", ruleFlow.ID, existing.RuleFlow.Version, ruleFlow.Version)
		}
		e.UnregisterDAG(ruleFlow.ID)
		e.mutex.Lock()
	}
	e.mutex.Unlock()

	dag, err := BuildDAGWithLoader(&ruleFlow, e.pluginLoader)
	if err != nil {
		return fmt.Errorf("failed to build DAG: %w", err)
	}

	if err := e.RegisterDAG(dag); err != nil {
		return err
	}

	// 如果规则启用，启动规则实例
	if ruleFlow.Enabled {

		if err := e.startRuleInstance(dag); err != nil {

			pkglog.Error("Failed to start rule instance for rule %s: %v", ruleFlow.ID, err)
			e.UnregisterDAG(ruleFlow.ID)
			return fmt.Errorf("failed to start rule instance: %w", err)
		} else {

		}
	} else {

	}

	return nil
}

func (e *Executor) lockRuleUpdate(ruleID string) func() {
	if ruleID == "" {
		return func() {}
	}

	actual, _ := e.ruleUpdateMu.LoadOrStore(ruleID, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ListRules 列出所有已注册的规则ID
func (e *Executor) ListRules() []string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	ruleIDs := make([]string, 0, len(e.dags))
	for ruleID := range e.dags {
		ruleIDs = append(ruleIDs, ruleID)
	}
	return ruleIDs
}

// GetRuntimeManager 获取 Runtime Manager（用于 API 暴露）
func (e *Executor) GetRuntimeManager() *runtime.Manager {
	return e.runtimeMgr
}

// ReplayDLQMessage replays one DLQ message back to the running dispatcher.
func (e *Executor) ReplayDLQMessage(msg *dlq.Message) error {
	if msg == nil {
		return fmt.Errorf("dlq message is nil")
	}
	e.mutex.RLock()
	exec, ok := e.executors[msg.RuleID]
	e.mutex.RUnlock()
	if !ok || exec == nil {
		return fmt.Errorf("no running executor found for rule %s", msg.RuleID)
	}
	return exec.ReplayDLQMessage(msg)
}

// SetStateManager 设置 State Manager
func (e *Executor) SetStateManager(stateMgr *state.StateManager) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.stateMgr = stateMgr
}

// SetDispatcherSecurityConfig sets dispatcher queue/limiter options for new rule instances.
func (e *Executor) SetDispatcherSecurityConfig(cfg *DispatcherSecurityConfig) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.dispatcherSec = cfg
}

// GetStats 获取统计信息
func (e *Executor) GetStats() map[string]interface{} {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["message_count"] = atomic.LoadInt64(&e.messageCount)
	stats["error_count"] = atomic.LoadInt64(&e.errorCount)
	stats["rule_count"] = len(e.dags)
	stats["pool_running"] = e.pool.Running()
	stats["pool_free"] = e.pool.Free()
	stats["pool_cap"] = e.pool.Cap()

	return stats
}
