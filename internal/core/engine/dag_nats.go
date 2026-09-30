package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	connectorruntime "github.com/punk-one/iot-rule-engine/internal/core/connector"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
	"github.com/punk-one/iot-rule-engine/internal/core/dlq"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin/rpc"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/state"
	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// DAGNATSExecutor DAG 基于 NATS 的执行器（使用 Dispatcher）
type DAGNATSExecutor struct {
	dag              *DAG
	runtimeMgr       *runtime.Manager
	instanceID       string
	stateMgr         *state.StateManager             // State Manager（用于 Stateful Plugin）
	disp             *dispatcher.Dispatcher          // 统一调度器
	pluginRPCs       map[string]*sdk.PluginRPCClient // nodeID -> PluginRPCClient（用于外部插件）
	resourceBus      *connectorruntime.ResourceBus
	resourceBindings map[string]string               // nodeID -> resource token
	resourceSubs     map[string]resourceSubscription // nodeID -> resource events subscription
}

type resourceSubscription struct {
	resourceID     string
	subscriptionID string
	events         <-chan sdk.ResourceStatusEvent
}

// DispatcherSecurityConfig controls dispatcher queue/limiter behavior.
type DispatcherSecurityConfig struct {
	MaxQueue              int
	RuleQPS               int
	NodeQPS               int
	DeduplicationWindowMs int
	MaxPayloadKB          int
	OverflowStrategy      string
}

// NewDAGNATSExecutor 创建基于 NATS 的 DAG 执行器
func NewDAGNATSExecutor(dag *DAG, runtimeMgr *runtime.Manager, instanceID string) *DAGNATSExecutor {
	return NewDAGNATSExecutorWithDeliveryAndSecurityConfig(dag, runtimeMgr, instanceID, nil, nil, nil)
}

// NewDAGNATSExecutorWithDeliveryConfig 创建带投递配置的 DAG 执行器
func NewDAGNATSExecutorWithDeliveryConfig(dag *DAG, runtimeMgr *runtime.Manager, instanceID string, stateMgr *state.StateManager, deliveryConfig *DeliveryConfig) *DAGNATSExecutor {
	return NewDAGNATSExecutorWithDeliveryAndSecurityConfig(dag, runtimeMgr, instanceID, stateMgr, deliveryConfig, nil)
}

// NewDAGNATSExecutorWithDeliveryAndSecurityConfig creates executor with dispatcher security config.
func NewDAGNATSExecutorWithDeliveryAndSecurityConfig(
	dag *DAG,
	runtimeMgr *runtime.Manager,
	instanceID string,
	stateMgr *state.StateManager,
	deliveryConfig *DeliveryConfig,
	securityConfig *DispatcherSecurityConfig,
) *DAGNATSExecutor {
	executor := &DAGNATSExecutor{
		dag:              dag,
		runtimeMgr:       runtimeMgr,
		instanceID:       instanceID,
		stateMgr:         stateMgr,
		pluginRPCs:       make(map[string]*sdk.PluginRPCClient),
		resourceBindings: make(map[string]string),
		resourceSubs:     make(map[string]resourceSubscription),
	}

	// 解析投递配置
	eventTimeout := 5 * time.Second
	serialDelivery := true
	deliveryMode := "best_effort"

	if deliveryConfig != nil {
		if deliveryConfig.EventTimeout != "" {
			if parsed, err := time.ParseDuration(deliveryConfig.EventTimeout); err == nil {
				eventTimeout = parsed
			}
		}
		if deliveryConfig.Mode != "" {
			deliveryMode = deliveryConfig.Mode
		}
		serialDelivery = deliveryConfig.SerialDelivery
	}

	// 创建带错误处理的 Dispatcher
	errorHandler := func(nodeID, eventID string, err error) {
		// 记录到 Runtime Model
		if runtimeMgr != nil {
			// nodeID 是 canvas node id，需要转换为 runtime node id
			runtimeNodeID := fmt.Sprintf("%s-%s", instanceID, nodeID)
			_ = runtimeMgr.RecordNodeError(instanceID, runtimeNodeID, eventID, err.Error())
		}
		pkglog.Error("Dispatcher error for node %s, event %s: %v", nodeID, eventID, err)
	}

	// 创建 Dispatcher（不使用回调）
	if securityConfig != nil {
		executor.disp = dispatcher.NewDispatcherWithSecurityConfig(
			dag.RuleID,
			eventTimeout,
			serialDelivery,
			deliveryMode,
			errorHandler,
			securityConfig.MaxQueue,
			securityConfig.RuleQPS,
			securityConfig.NodeQPS,
			securityConfig.DeduplicationWindowMs,
			securityConfig.MaxPayloadKB,
			securityConfig.OverflowStrategy,
		)
	} else {
		executor.disp = dispatcher.NewDispatcherWithConfig(
			dag.RuleID,
			eventTimeout,
			serialDelivery,
			deliveryMode,
			errorHandler,
		)
	}

	// 创建 RuntimeObserver 并注入到 Dispatcher Hooks
	observer := runtime.NewRuntimeObserver(instanceID, runtimeMgr, dag.RuleFlow.DebugMode)
	executor.disp.SetHooks(dispatcher.DispatcherHooks{
		CapturePayload: dag.RuleFlow.DebugMode,
		OnNodeReceive:  observer.OnNodeReceive,
		OnNodeEmit:     observer.OnNodeEmit,
		OnTaskDone:     observer.OnTaskDone,
		OnNodeMetric:   observer.OnNodeMetric,
		OnNodeAck:      observer.OnNodeAck,
	})

	return executor
}

// DeliveryConfig 投递配置
type DeliveryConfig struct {
	Mode           string
	EventTimeout   string
	SerialDelivery bool
}

// NewDAGNATSExecutorWithState 创建带 State 支持的 DAG 执行器
func NewDAGNATSExecutorWithState(dag *DAG, runtimeMgr *runtime.Manager, instanceID string, stateMgr *state.StateManager) *DAGNATSExecutor {
	return NewDAGNATSExecutorWithDeliveryAndSecurityConfig(dag, runtimeMgr, instanceID, stateMgr, nil, nil)
}

// Start 启动 DAG 执行器（启动所有插件并订阅 NATS Topic）
func (e *DAGNATSExecutor) Start(ctx context.Context) error {
	js := bus.GetJetStream()
	if js == nil {
		return fmt.Errorf("JetStream not initialized")
	}

	// 使用 Dispatcher 统一调度
	// 1. 创建并启动 Dispatcher（统一订阅 Edge Topic）
	// 2. 为每个插件创建 RuntimeContext 并启动插件
	// 3. 将所有插件注册到 Dispatcher

	// 构建物理边和逻辑路由表。
	edges := collectDispatcherEdges(e.dag.RuleFlow)
	logicalRouters := collectLogicalRouters(e.dag.RuleFlow)

	// 注册规则到 Dispatcher
	e.disp.RegisterRule(e.dag.RuleID, edges)
	pkglog.Info("Registered rule %s to dispatcher with %d edges", e.dag.RuleID, len(edges))
	for nodeID, routerEdges := range logicalRouters {
		e.disp.RegisterRouter(nodeID, routerEdges)
	}
	if len(logicalRouters) > 0 {
		pkglog.Info("Registered %d logical routers for rule %s", len(logicalRouters), e.dag.RuleID)
	}

	// 启动 Dispatcher
	if err := e.disp.Start(); err != nil {
		return fmt.Errorf("failed to start dispatcher: %w", err)
	}
	pkglog.Info("Dispatcher started for rule %s", e.dag.RuleID)

	pkglog.Info("DAG NATS executor started for rule %s (dispatcher-enabled)", e.dag.RuleID)
	return nil
}

// RegisterPlugin 注册插件到 Dispatcher（供 Executor 调用）
// Rule 级 Fail-Fast：插件加载失败时返回错误，Rule 不进入 Snapshot
func (e *DAGNATSExecutor) RegisterPlugin(nodeID string, plugin sdk.Plugin) error {
	// 注册插件到 Dispatcher
	pluginClient := e.createPluginClient(nodeID, plugin)
	if pluginClient == nil {
		// 插件加载失败：返回错误（Rule 级 Fail-Fast）
		return fmt.Errorf("plugin load failed for node %s (rule: %s) - rule will not be activated", nodeID, e.dag.RuleID)
	}

	// 注册到 RuleRuntime
	e.disp.RegisterPluginToRule(e.dag.RuleID, nodeID, pluginClient)
	pkglog.Debug("Registered plugin %s to dispatcher (rule: %s)", nodeID, e.dag.RuleID)
	return nil
}

// executePluginViaNATS 通过 NATS 执行插件
// 注意：插件应该通过 RuntimeContext 和 NATS 交互
// 这个方法主要用于处理从 Edge Topic 接收的消息
// Source 插件应该主动从 Gateway 或外部系统获取数据，不需要通过这个方法处理
func (e *DAGNATSExecutor) executePluginViaNATS(ctx context.Context, nodeID string, plugin sdk.Plugin, msg *schema.RuleMessage) error {
	// 更新节点指标
	// nodeID 是 canvas node id，需要转换为 runtime node id
	runtimeNodeID := fmt.Sprintf("%s-%s", e.instanceID, nodeID)
	e.runtimeMgr.IncrementNodeMetrics(e.instanceID, runtimeNodeID, 1, 0, 0)

	// 将 RuleMessage 转换为 Event
	event := convertRuleMessageToEvent(msg)

	// 获取插件信息
	pluginInfo := plugin.Info()

	// 根据插件类型处理
	switch pluginInfo.Type {
	case sdk.PluginTypeSource:
		// Source 插件应该主动从 Gateway 或外部系统获取数据
		// 不应该通过 Edge Topic 接收消息
		// 如果收到 Source 插件的消息，说明架构有问题
		pkglog.Error("Source plugin %s should not receive messages via Edge Topic. Source plugins must actively fetch data from Gateway or external systems.", nodeID)
		return fmt.Errorf("source plugin %s should not receive messages via Edge Topic", nodeID)

	case sdk.PluginTypeProcessor, sdk.PluginTypeSink:
		// Processor 和 Sink 插件应该通过 RuntimeContext.Subscribe() 接收事件
		// 这里需要将事件发布到对应的 NATS Topic，让插件订阅处理
		// 注意：实际的插件执行应该已经在 Start() 时通过 RuntimeContext 设置好了

		// 获取节点配置
		nodeConfig := e.dag.GetNodeConfig(nodeID)
		if nodeConfig == nil {
			return fmt.Errorf("node config not found for %s", nodeID)
		}

		// 使用语义级 API 发布到 Edge Topic
		// 注意：事件应该通过 Dispatcher 分发
		// 但为了向后兼容，这里仍然支持直接发布（使用语义级 API）
		if len(nodeConfig.Wires) > 0 {
			// 将 Event 编码为字节数组
			data, err := sdk.EncodeEvent(event)
			if err != nil {
				pkglog.Error("Failed to encode event: %v", err)
				return fmt.Errorf("failed to encode event: %w", err)
			}

			// 使用语义级 API 发布到 Edge Topic
			for _, nextNodeID := range nodeConfig.Wires {
				if err := bus.PublishRuleEdge(e.dag.RuleID, nodeID, nextNodeID, data); err != nil {
					pkglog.Error("Failed to publish rule edge for rule %s, from %s to %s: %v", e.dag.RuleID, nodeID, nextNodeID, err)
					continue
				}

				// 更新 Edge 指标
				// edgeID 是 canvas edge id，需要转换为 runtime edge id
				canvasEdgeID := fmt.Sprintf("%s-%s", nodeID, nextNodeID)
				runtimeEdgeID := fmt.Sprintf("%s-%s", e.instanceID, canvasEdgeID)
				e.runtimeMgr.IncrementEdgeMetrics(e.instanceID, runtimeEdgeID, 1, 0, 0)

				pkglog.Debug("Published event from node %s to %s via rule.edge.%s.%s.%s", nodeID, nextNodeID, e.dag.RuleID, nodeID, nextNodeID)
			}
		}

		// 更新节点指标
		// nodeID 是 canvas node id，需要转换为 runtime node id
		runtimeNodeID := fmt.Sprintf("%s-%s", e.instanceID, nodeID)
		e.runtimeMgr.IncrementNodeMetrics(e.instanceID, runtimeNodeID, 0, 1, 0)

		// 如果是 Debug 模式，发送 Trace 消息
		if msg.IsDebug {
			e.sendTrace(nodeID, msg, nil, "success")
		}

		return nil

	default:
		return fmt.Errorf("unsupported plugin type: %s", pluginInfo.Type)
	}
}

// convertRuleMessageToEvent 将 RuleMessage 转换为 Event
func convertRuleMessageToEvent(msg *schema.RuleMessage) sdk.Event {
	var payload map[string]interface{}
	if len(msg.Payload) > 0 {
		payload = msg.Payload[0]
	} else {
		payload = make(map[string]interface{})
	}

	if msg.Metadata == nil {
		msg.Metadata = make(map[string]string)
	}

	return sdk.Event{
		ID:        msg.ID,
		Timestamp: time.Unix(0, msg.Timestamp),
		Payload:   payload,
		Metadata:  msg.Metadata,
	}
}

// convertEventToRuleMessage 将 Event 转换回 RuleMessage（用于兼容）
func convertEventToRuleMessage(event sdk.Event, originalMsg *schema.RuleMessage) *schema.RuleMessage {
	return &schema.RuleMessage{
		ID:        event.ID,
		Timestamp: event.Timestamp.UnixNano(),
		Payload:   []map[string]interface{}{event.Payload},
		BatchSize: 1,
		Metadata:  event.Metadata,
		TraceID:   sdk.GetTraceID(event),
		RuleID:    originalMsg.RuleID,
		SourceID:  originalMsg.SourceID,
		IsDryRun:  originalMsg.IsDryRun,
		IsDebug:   originalMsg.IsDebug,
	}
}

// findSourceNodes 查找所有 Source 节点（入度为 0）
func (e *DAGNATSExecutor) findSourceNodes() []string {
	var sourceNodeIDs []string

	// 计算每个节点的入度
	inDegree := make(map[string]int)
	for _, nodeConfig := range e.dag.RuleFlow.Nodes {
		inDegree[nodeConfig.ID] = 0
	}

	// 遍历所有节点拓扑边，计算入度
	for _, edge := range collectTopologyEdges(e.dag.RuleFlow) {
		inDegree[edge.To]++
	}

	// 找到入度为 0 的节点
	for nodeID, degree := range inDegree {
		if degree == 0 {
			// 验证插件是否存在
			if _, exists := e.dag.GetPlugin(nodeID); exists {
				sourceNodeIDs = append(sourceNodeIDs, nodeID)
			}
		}
	}

	return sourceNodeIDs
}

// InjectRuleMessage transfers ownership of an ingress message to the running
// dispatcher. Each root branch receives an independent Event clone.
func (e *DAGNATSExecutor) InjectRuleMessage(msg *schema.RuleMessage) error {
	if msg == nil {
		return fmt.Errorf("message cannot be nil")
	}
	roots := e.findSourceNodes()
	if len(roots) == 0 {
		return fmt.Errorf("rule %s has no ingress node", e.dag.RuleID)
	}
	base := convertRuleMessageToEvent(msg)
	base.RuleID = e.dag.RuleID
	if msg.TraceID != "" {
		sdk.SetTraceID(&base, msg.TraceID)
	}
	for _, rootID := range roots {
		branch := base.Clone()
		if err := e.disp.OnPluginEmit(e.dag.RuleID, rootID, &branch); err != nil {
			return fmt.Errorf("dispatch ingress from %s: %w", rootID, err)
		}
	}
	return nil
}

// getEdgeTopic 获取 Edge 的 NATS Topic（仅用于元数据/日志，不用于实际发布）
// 实际发布应使用 bus.PublishRuleEdge()
// 此方法仅用于构建 Edge Instance 的 Topic 字段（元数据）
func (e *DAGNATSExecutor) getEdgeTopic(fromNodeID, toNodeID string) string {
	return fmt.Sprintf("rule.edge.%s.%s.%s", e.dag.RuleID, fromNodeID, toNodeID)
}

// sendTrace 发送 Trace 消息到 sys.trace 主题
func (e *DAGNATSExecutor) sendTrace(nodeID string, inputMsg, outputMsg *schema.RuleMessage, status string) {
	if e.dag.RuleFlow.ID == "" {
		return
	}

	// 构建 Trace 消息
	observedAt := time.Now()
	traceMsg := map[string]interface{}{
		"instance_id":     e.instanceID,
		"rule_id":         e.dag.RuleFlow.ID,
		"node_id":         nodeID,
		"node_type":       e.dag.GetNodeType(nodeID),
		"phase":           "done",
		"status":          status,
		"timestamp":       observedAt,
		"observed_at":     observedAt,
		"event_timestamp": time.Unix(0, inputMsg.Timestamp),
		"trace_id":        inputMsg.TraceID,
		"event_id":        inputMsg.ID,
		"payload":         inputMsg.Payload, // 添加 Payload
	}

	traceMsg["message"] = buildTraceMessage(inputMsg.Payload, status)

	// 如果输出消息不为空，添加输出信息
	if outputMsg != nil {
		traceMsg["output_batch_size"] = outputMsg.BatchSize
		traceMsg["from_node_id"] = outputMsg.FromNodeID
	} else {
		traceMsg["output_batch_size"] = 0
	}

	// 发送到 sys.trace 主题
	traceSubject := fmt.Sprintf("sys.trace.%s.%s", e.dag.RuleFlow.ID, nodeID)
	js := bus.GetJetStream()
	if js != nil {
		if err := js.Publish(traceSubject, traceMsg); err != nil {
			pkglog.Error("Failed to publish trace to %s: %v", traceSubject, err)
		} else {
			pkglog.Debug("Published trace to %s", traceSubject)
		}
	}
}

func buildTraceMessage(payload interface{}, status string) string {
	if payload == nil {
		return fmt.Sprintf("status=%s payload_bytes=0", status)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("status=%s payload_bytes=0 payload_type=%T", status, payload)
	}
	return fmt.Sprintf("status=%s payload_bytes=%d payload_type=%T", status, len(payloadJSON), payload)
}

// CreateRuntimeContext 为节点创建 RuntimeContext
func (e *DAGNATSExecutor) CreateRuntimeContext(nodeID string) sdk.RuntimeContext {
	ctx := context.Background()
	// 创建带 rule_id 和 node_id 的上下文
	runtimeCtx := context.WithValue(ctx, "rule_id", e.dag.RuleID)
	runtimeCtx = context.WithValue(runtimeCtx, "node_id", nodeID)

	var sdkStateMgr sdk.StateManager
	if e.stateMgr != nil {
		sdkStateMgr = sdk.NewStateManager(e.dag.RuleID, nodeID, e.stateMgr)
	}

	// 获取节点配置
	nodeConfig := e.dag.GetNodeConfig(nodeID)
	if nodeConfig == nil {
		pkglog.Error("Node config not found for %s", nodeID)
	}

	// 检查插件是否需要 State 支持
	// 根据插件类型判断：category 为 "stateful" 的插件需要 State
	// publishFunc 已移除，当前通过 pluginContext 直接使用 Dispatcher。

	// 为外部插件创建 EngineRPC（在 Start 之前创建）
	// 检查是否是外部插件：外部插件是通过 plugin.Loader 加载的
	// 我们通过检查 DAG 的 loader 和插件类型来判断
	// 注意：这里我们暂时不创建 EngineRPC，而是在 pluginWrapper.Start 中创建
	// 但我们需要在 createPluginClient 中能够获取到 EngineRPC
	// 所以我们在 pluginWrapper.Start 中保存 EngineRPC 到 e.pluginRPCs
	// 这里先不处理，等待 pluginWrapper.Start 调用
	// TODO: 优化架构，让 EngineRPC 的创建和保存更清晰

	// 不再创建 Subscribe 函数
	// 所有 Edge Topic 订阅由 Dispatcher 统一管理
	// 插件通过 Dispatcher 接收事件（外部插件通过 RPC，内置插件通过 OnEvent）

	// 在 Engine 端创建 EngineRPCServer（注入 dispatcher）
	// 获取下游节点列表（用于 EngineRPCServer）
	var toNodeIDs []string
	if nodeConfig != nil {
		seen := make(map[string]struct{})
		for _, edge := range collectNodeTopologyEdges(nodeConfig) {
			if _, exists := seen[edge.To]; exists {
				continue
			}
			seen[edge.To] = struct{}{}
			toNodeIDs = append(toNodeIDs, edge.To)
		}
	}

	runtimeNodeID := fmt.Sprintf("%s-%s", e.instanceID, nodeID)
	healthReporter := newNodeHealthReporter(e.runtimeMgr, e.instanceID, runtimeNodeID)
	connectorClient, resourceEvents := e.buildConnectorBinding(nodeID, nodeConfig)

	// 获取 NATS 连接和 Logger
	js := bus.GetJetStream()
	if js == nil {
		pkglog.Error("JetStream not initialized, cannot create EngineRPCServer for node %s (rule: %s) - this is an Engine-level error", nodeID, e.dag.RuleID)
		// 返回一个空的 RuntimeContext，但后续插件加载会失败（Rule 级 Fail-Fast）
		// 这是 Engine 级别的错误，应该在 Engine 启动时检查
		return NewPluginContext(
			runtimeCtx,
			e.dag.RuleID,
			nodeID,
			e.disp,
			sdkStateMgr,
			connectorClient,
			resourceEvents,
			func(*sdk.PluginRPCClient) {},
			nil,
			healthReporter,
			func(string) {},
		)
	}
	nc := js.GetConn()
	if nc == nil {
		pkglog.Error("NATS connection is nil, cannot create EngineRPCServer for node %s (rule: %s) - this is an Engine-level error", nodeID, e.dag.RuleID)
		// 返回一个空的 RuntimeContext，但后续插件加载会失败（Rule 级 Fail-Fast）
		return NewPluginContext(
			runtimeCtx,
			e.dag.RuleID,
			nodeID,
			e.disp,
			sdkStateMgr,
			connectorClient,
			resourceEvents,
			func(*sdk.PluginRPCClient) {},
			nil,
			healthReporter,
			func(string) {},
		)
	}
	pluginID := ""
	if nodeConfig != nil {
		pluginID = nodeConfig.Type
		if nodeConfig.Plugin != "" {
			pluginID = nodeConfig.Plugin
		}
	}
	logger := pkglog.NewPluginSDKLogger(e.dag.RuleID, pluginID, nodeID)

	// 创建 EngineRPCServer（在 Engine 端创建，注入 dispatcher）
	// 传递 Dispatcher 作为 PluginRegistry（用于 RPC 鉴权）
	engineRPCServer := rpc.NewEngineRPCServerWithRegistry(
		nc,
		logger,
		e.dag.RuleID,
		nodeID,
		toNodeIDs,
		e.disp,
		e.stateMgr,
		e.disp,
		healthReporter,
		connectorClient,
		resourceEvents,
	)
	pkglog.Debug("Created EngineRPCServer for node %s (rule: %s) in Engine", nodeID, e.dag.RuleID)

	// 创建保存 PluginRPCClient 的回调函数
	savePluginRPCCB := func(pluginRPC *sdk.PluginRPCClient) {
		e.pluginRPCs[nodeID] = pluginRPC
		if e.disp != nil {
			e.disp.AuthorizePluginStartupRPC(e.dag.RuleID, nodeID)
		}
		pkglog.Debug("Saved PluginRPCClient for node %s via callback", nodeID)
	}

	// Metrics are now updated in RuntimeObserver (via Dispatcher Hooks)
	// This handles both direct pluginContext.Publish calls and RPC-based EngineRPC.Emit calls

	// 创建 RuntimeContext（使用自定义 pluginContext）
	// 将 EngineRPCServer 传递给 pluginContext，供 loader.go 使用
	// 外部插件通过 RPC -> EngineRPC -> dispatcher.OnPluginEmit 路由
	return NewPluginContext(
		runtimeCtx,
		e.dag.RuleID,
		nodeID,
		e.disp,
		sdkStateMgr,
		connectorClient,
		resourceEvents,
		savePluginRPCCB,
		engineRPCServer,
		healthReporter,
		nil,
	)
}

func (e *DAGNATSExecutor) buildConnectorBinding(nodeID string, nodeConfig *schema.NodeConfig) (sdk.ConnectorClient, <-chan sdk.ResourceStatusEvent) {
	if e.resourceBus == nil || nodeConfig == nil || nodeConfig.Config == nil {
		return connectorruntime.NewBoundClient(nil, "", ""), nil
	}

	resourceID, ok := connectorRefFromNodeConfig(nodeConfig)
	if !ok {
		return connectorruntime.NewBoundClient(nil, "", ""), nil
	}
	bindingOptions := connectorBindingOptionsFromNodeConfig(nodeConfig)

	if token, ok := e.resourceBindings[nodeID]; ok && token != "" {
		events := e.ensureResourceSubscription(nodeID, resourceID)
		return connectorruntime.NewBoundClientWithDefaults(
			e.resourceBus,
			token,
			resourceID,
			bindingOptions.DefaultPriority,
			bindingOptions.UsageWeight,
		), events
	}

	token, err := e.resourceBus.BindWithOptions(context.Background(), resourceID, bindingOptions)
	if err != nil {
		pkglog.Error("Failed to bind connector resource %s for node %s (rule: %s): %v", resourceID, nodeID, e.dag.RuleID, err)
		return connectorruntime.NewBoundClient(nil, "", ""), nil
	}

	e.resourceBindings[nodeID] = token
	events := e.ensureResourceSubscription(nodeID, resourceID)
	return connectorruntime.NewBoundClientWithDefaults(
		e.resourceBus,
		token,
		resourceID,
		bindingOptions.DefaultPriority,
		bindingOptions.UsageWeight,
	), events
}

func (e *DAGNATSExecutor) ensureResourceSubscription(nodeID, resourceID string) <-chan sdk.ResourceStatusEvent {
	if e.resourceSubs == nil {
		e.resourceSubs = make(map[string]resourceSubscription)
	}
	if sub, ok := e.resourceSubs[nodeID]; ok && sub.resourceID == resourceID && sub.events != nil {
		return sub.events
	}

	subID, events, err := e.resourceBus.Subscribe(resourceID)
	if err != nil {
		pkglog.Warn("Failed to subscribe resource events for node %s resource %s (rule: %s): %v", nodeID, resourceID, e.dag.RuleID, err)
		return nil
	}
	e.resourceSubs[nodeID] = resourceSubscription{
		resourceID:     resourceID,
		subscriptionID: subID,
		events:         events,
	}
	return events
}

func connectorBindingOptionsFromNodeConfig(nodeConfig *schema.NodeConfig) connectorruntime.BindingOptions {
	opts := connectorruntime.BindingOptions{
		UsageWeight: 1,
	}
	if nodeConfig == nil || nodeConfig.Config == nil {
		return opts
	}
	if raw, ok := nodeConfig.Config["priority"].(string); ok {
		opts.DefaultPriority = raw
	}
	if raw, ok := toPositiveInt(nodeConfig.Config["usage_weight"]); ok {
		opts.UsageWeight = raw
	}
	if raw, ok := toPositiveInt(nodeConfig.Config["max_usage_weight"]); ok {
		opts.MaxUsageWeight = raw
	}
	return opts
}

func toPositiveInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int32:
		return int(typed), typed > 0
	case int64:
		return int(typed), typed > 0
	case float64:
		return int(typed), typed > 0
	default:
		return 0, false
	}
}

func connectorRefFromNodeConfig(nodeConfig *schema.NodeConfig) (string, bool) {
	if nodeConfig == nil || nodeConfig.Config == nil {
		return "", false
	}
	raw, ok := nodeConfig.Config["connector_ref"]
	if !ok {
		return "", false
	}
	ref, ok := raw.(string)
	if !ok || ref == "" {
		return "", false
	}
	return ref, true
}

func (e *DAGNATSExecutor) SetResourceBus(resourceBus *connectorruntime.ResourceBus) {
	e.resourceBus = resourceBus
}

// createPluginClient 创建 PluginClient（所有插件都按外部插件路径接入）
func (e *DAGNATSExecutor) createPluginClient(nodeID string, plugin sdk.Plugin) client.PluginClient {
	// 所有插件都应该是外部插件
	// 检查 PluginRPCClient 是否存在
	pluginRPC, exists := e.pluginRPCs[nodeID]
	if !exists || pluginRPC == nil {
		// 插件加载失败：返回错误（Rule 级 Fail-Fast）
		// 这通常发生在 pluginWrapper.Start() 还没有调用 SavePluginRPCClient 回调
		// 或者在 pluginRPCs 中保存之前就调用了 RegisterPlugin
		// 理论上，pluginWrapper.Start() 是同步的，SavePluginRPCClient 应该已经调用
		pkglog.Error("PluginRPCClient not found for node %s (rule: %s) - plugin may not have started properly or SavePluginRPCClient not called yet", nodeID, e.dag.RuleID)
		return nil // 返回 nil 表示插件加载失败
	}

	// 外部插件：使用 ExternalPluginClient
	pkglog.Debug("Creating ExternalPluginClient for node %s (rule: %s)", nodeID, e.dag.RuleID)
	return client.NewExternalPluginClient(nodeID, plugin, pluginRPC)
}

// SetPluginRPC 设置插件的 RPCClient（供兼容路径调用）。
func (e *DAGNATSExecutor) SetPluginRPC(nodeID string, pluginRPC *sdk.PluginRPCClient) {
	e.pluginRPCs[nodeID] = pluginRPC
	pkglog.Debug("Set plugin RPC client for node %s", nodeID)
}

// ReplayDLQMessage replays one DLQ message back to its target node.
func (e *DAGNATSExecutor) ReplayDLQMessage(msg *dlq.Message) error {
	if msg == nil {
		return fmt.Errorf("dlq message is nil")
	}
	if e.disp == nil {
		return fmt.Errorf("dispatcher not initialized for rule %s", e.dag.RuleID)
	}
	return e.disp.ReplayEventToNode(msg.RuleID, msg.FromNodeID, msg.NodeID, msg.Event)
}

// Stop 停止 DAG 执行器
func (e *DAGNATSExecutor) Stop() error {
	if e.resourceBus != nil {
		for nodeID, sub := range e.resourceSubs {
			if sub.subscriptionID != "" && sub.resourceID != "" {
				e.resourceBus.Unsubscribe(sub.resourceID, sub.subscriptionID)
			}
			delete(e.resourceSubs, nodeID)
		}
		for nodeID, token := range e.resourceBindings {
			if token == "" {
				continue
			}
			if err := e.resourceBus.Release(context.Background(), token); err != nil {
				pkglog.Warn("Failed to release connector binding for node %s (rule: %s): %v", nodeID, e.dag.RuleID, err)
			}
		}
		e.resourceBindings = make(map[string]string)
		e.resourceSubs = make(map[string]resourceSubscription)
	}

	// 注销规则
	if e.disp != nil {
		e.disp.UnregisterRule(e.dag.RuleID)
	}

	// 停止 Dispatcher（会自动停止所有插件）
	if e.disp != nil {
		e.disp.Stop()
	}

	pkglog.Info("DAG NATS executor stopped for rule %s", e.dag.RuleID)
	return nil
}
