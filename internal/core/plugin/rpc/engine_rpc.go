package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/state"
)

// DispatcherInterface Dispatcher 接口
// 用于 EngineRPCServer 调用 Dispatcher 的方法
type DispatcherInterface interface {
	OnPluginEmit(ruleID, fromNodeID string, event *sdk.Event) error
	OnPluginEmitBatch(ruleID, fromNodeID string, events []sdk.Event) error
	PublishEventToNodes(fromNodeID string, toNodeIDs []string, event sdk.Event) error
	OnPluginMetric(ruleID, nodeID string, name string, value float64, labels map[string]string)
	OnPluginAck(ruleID, nodeID string, eventID string)
}

// PluginRegistry 插件注册表接口（用于 RPC 鉴权）
type PluginRegistry interface {
	IsPluginRegistered(ruleID, nodeID string) bool
}

type HealthUpdateSink interface {
	ApplyHealthUpdate(update sdk.ReportHealthArgs) error
}

// EngineRPCServer 引擎端 RPC 服务端实现
// 实现 sdk.EngineRPC 接口
// 添加 RPC 鉴权支持
type EngineRPCServer struct {
	nc         *nats.Conn
	logger     sdk.Logger
	ruleID     string
	nodeID     string
	toNodeIDs  []string            // 保存下游节点列表（从 DAG 配置获取）
	dispatcher DispatcherInterface // Dispatcher 引用（用于发布事件）
	stateMgr   *state.StateManager // Stateful 插件状态后端
	registry   PluginRegistry      // 插件注册表（用于 RPC 鉴权）
	health     HealthUpdateSink
	connector  sdk.ConnectorClient
	events     <-chan sdk.ResourceStatusEvent
	streamMu   sync.Mutex
	streams    map[string]sdk.StreamConsumer
}

// StateKeyArgs 是状态 RPC 的兼容参数。
// SDK v1.5.2 已移除对应导出类型，这里在 engine 侧本地保留。
type StateKeyArgs struct {
	Key string
}

// StateGetReply 是状态 RPC 的兼容返回值。
type StateGetReply struct {
	Value []byte
	Found bool
}

// StateSetArgs 是状态 RPC 的兼容参数。
type StateSetArgs struct {
	Key   string
	Value []byte
}

// StateSetWithTTLArgs 是带 TTL 的状态写入参数。
type StateSetWithTTLArgs struct {
	Key      string
	Value    []byte
	TTLNanos int64
}

type ConnectorExecuteRPCReply struct {
	Response sdk.ConnectorResponse
	Error    string
}

type ConnectorOpenStreamRPCReply struct {
	Response sdk.StreamOpenResponse
	Error    string
}

type ConnectorReceiveStreamRPCReply struct {
	Response sdk.StreamReceiveResponse
	Error    string
}

type ConnectorAckStreamRPCReply struct {
	Error string
}

type ConnectorNackStreamRPCReply struct {
	Error string
}

type ConnectorGrantCreditsRPCReply struct {
	Error string
}

type ConnectorCloseStreamRPCReply struct {
	Error string
}

type CurrentResourceStatusRPCArgs struct {
	ResourceRef string
}

type CurrentResourceStatusRPCReply struct {
	Event sdk.ResourceStatusEvent
	Found bool
}

type NextResourceEventRPCArgs struct {
	TimeoutMS int
}

type NextResourceEventRPCReply struct {
	Event sdk.ResourceStatusEvent
	OK    bool
}

// NewEngineRPCServer 创建引擎端 RPC 服务端
func NewEngineRPCServer(nc *nats.Conn, logger sdk.Logger, ruleID, nodeID string, toNodeIDs []string, dispatcher DispatcherInterface, stateMgr *state.StateManager, connector sdk.ConnectorClient, events <-chan sdk.ResourceStatusEvent) *EngineRPCServer {
	return NewEngineRPCServerWithRegistry(nc, logger, ruleID, nodeID, toNodeIDs, dispatcher, stateMgr, nil, nil, connector, events)
}

// NewEngineRPCServerWithRegistry 创建带注册表的引擎端 RPC 服务端
func NewEngineRPCServerWithRegistry(nc *nats.Conn, logger sdk.Logger, ruleID, nodeID string, toNodeIDs []string, dispatcher DispatcherInterface, stateMgr *state.StateManager, registry PluginRegistry, health HealthUpdateSink, connector sdk.ConnectorClient, events <-chan sdk.ResourceStatusEvent) *EngineRPCServer {
	return &EngineRPCServer{
		nc:         nc,
		logger:     logger,
		ruleID:     ruleID,
		nodeID:     nodeID,
		toNodeIDs:  toNodeIDs,
		dispatcher: dispatcher,
		stateMgr:   stateMgr,
		registry:   registry,
		health:     health,
		connector:  connector,
		events:     events,
		streams:    make(map[string]sdk.StreamConsumer),
	}
}

func (e *EngineRPCServer) ensureAuthorized() error {
	if e.registry != nil && !e.registry.IsPluginRegistered(e.ruleID, e.nodeID) {
		pkglog.Warn("EngineRPC: unauthorized RPC call (rule=%s, node=%s)", e.ruleID, e.nodeID)
		return fmt.Errorf("unauthorized RPC call: plugin not registered (rule=%s, node=%s)", e.ruleID, e.nodeID)
	}
	return nil
}

// Emit 发布事件到下游节点
// 实现 sdk.EngineRPC 接口
// 使用保存的下游节点列表
// 即使 toNodeIDs 为空，也要调用 dispatcher.OnPluginEmit 来更新指标
func (e *EngineRPCServer) Emit(event sdk.Event) error {
	// 即使 toNodeIDs 为空，也要调用 EmitWithTargets 来更新指标
	// EmitWithTargets 会检查 toNodeIDs 是否为空，如果为空但仍然调用 dispatcher.OnPluginEmit
	return e.EmitWithTargets(event, e.toNodeIDs)
}

// EmitBatch 批量发布事件
func (e *EngineRPCServer) EmitBatch(events []sdk.Event) error {
	if e.dispatcher != nil {
		if err := e.dispatcher.OnPluginEmitBatch(e.ruleID, e.nodeID, events); err != nil {
			pkglog.Error("Failed to emit batch events via dispatcher: %v", err)
			return err
		}
	}
	return nil
}

// EmitWithTargets 发布事件到指定的下游节点
// 实现 sdk.EngineRPC 接口
// 优先使用 Dispatcher.OnPluginEmit，如果没有 dispatcher 则回退到直接发布
// 添加 RPC 鉴权
func (e *EngineRPCServer) EmitWithTargets(event sdk.Event, toNodeIDs []string) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}

	// 即使没有下游节点，也要调用 dispatcher.OnPluginEmit 来更新指标
	// 因为插件已经发出了事件，即使没有下游节点，也应该更新 EventsOut 指标
	if len(toNodeIDs) == 0 {
		// 没有下游节点（Sink 节点），但仍然需要调用 dispatcher.OnPluginEmit 来更新指标
		if e.dispatcher != nil {
			// 调用 dispatcher.OnPluginEmit 来更新指标（即使没有下游节点）
			if err := e.dispatcher.OnPluginEmit(e.ruleID, e.nodeID, &event); err != nil {
				pkglog.Error("Failed to emit event via dispatcher (no downstream nodes): %v", err)
				return err
			}
		}
		return nil
	}

	// 优先使用 Dispatcher.OnPluginEmit
	if e.dispatcher != nil {
		// 使用 Dispatcher 路由事件
		// 注意：OnPluginEmit 会根据 ruleID 和 fromNodeID 自动查找下游节点
		// 但这里我们明确指定了 toNodeIDs，所以需要确保 Dispatcher 的 RuleRuntime 中有正确的 edges
		// 为了兼容，我们仍然调用 OnPluginEmit，它会在 RuleRuntime 中查找 edges
		if err := e.dispatcher.OnPluginEmit(e.ruleID, e.nodeID, &event); err != nil {
			pkglog.Error("Failed to emit event via dispatcher: %v", err)
			if strings.Contains(err.Error(), "backpressure") {
				// 背压错误必须原样返回给上游，避免回退直发绕过流控。
				return err
			}
			// 回退到直接发布
			return e.emitDirectly(event, toNodeIDs)
		}
		pkglog.Debug("EngineRPC emitted event via dispatcher (eventID: %s)", event.ID)
		return nil
	}

	// 回退到直接发布（向后兼容）
	return e.emitDirectly(event, toNodeIDs)
}

// emitDirectly 直接发布事件到 NATS（向后兼容方法）
// 使用语义级 API
func (e *EngineRPCServer) emitDirectly(event sdk.Event, toNodeIDs []string) error {
	// 编码事件
	data, err := sdk.EncodeEvent(event)
	if err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	// 发布到所有下游节点的 Edge Topic
	// 使用语义级 API，禁止裸 subject
	for _, toNodeID := range toNodeIDs {
		if err := bus.PublishRuleEdge(e.ruleID, e.nodeID, toNodeID, data); err != nil {
			pkglog.Error("Failed to publish event to rule edge (rule=%s, from=%s, to=%s): %v", e.ruleID, e.nodeID, toNodeID, err)
			continue
		}
		pkglog.Debug("EngineRPC published event to rule edge (rule=%s, from=%s, to=%s, eventID=%s)", e.ruleID, e.nodeID, toNodeID, event.ID)
	}

	return nil
}

func (e *EngineRPCServer) Log(level sdk.LogLevel, msg string, fields map[string]interface{}) {
	// 将 fields map 转换为 Field 切片
	var fieldSlice []sdk.Field
	for k, v := range fields {
		fieldSlice = append(fieldSlice, sdk.Field{Key: k, Value: v})
	}

	// 调用 logger 的对应方法
	switch level {
	case sdk.LogLevelDebug:
		e.logger.Debug(msg, fieldSlice...)
	case sdk.LogLevelInfo:
		e.logger.Info(msg, fieldSlice...)
	case sdk.LogLevelWarn:
		e.logger.Warn(msg, fieldSlice...)
	case sdk.LogLevelError:
		e.logger.Error(msg, fieldSlice...)
	default:
		e.logger.Info(msg, fieldSlice...)
	}
}

// LogBatch 批量记录日志
func (e *EngineRPCServer) LogBatch(level sdk.LogLevel, messages []string, fields []map[string]interface{}) {
	for i, msg := range messages {
		var f map[string]interface{}
		if i < len(fields) {
			f = fields[i]
		}
		e.Log(level, msg, f)
	}
}

// IncCounter 增加计数器
func (e *EngineRPCServer) IncCounter(name string, labels map[string]string) {
	if e.dispatcher != nil {
		e.dispatcher.OnPluginMetric(e.ruleID, e.nodeID, name, 1, labels)
	}
}

// Observe 观测指标
func (e *EngineRPCServer) Observe(name string, value float64, labels map[string]string) {
	if e.dispatcher != nil {
		e.dispatcher.OnPluginMetric(e.ruleID, e.nodeID, name, value, labels)
	}
}

func (e *EngineRPCServer) ReportHealth(args sdk.ReportHealthArgs) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.health == nil {
		return nil
	}
	return e.health.ApplyHealthUpdate(args)
}

// Ack 确认消息
func (e *EngineRPCServer) Ack(eventID string) error {
	if e.dispatcher != nil {
		e.dispatcher.OnPluginAck(e.ruleID, e.nodeID, eventID)
	}
	return nil
}

func (e *EngineRPCServer) PublishAck(ack sdk.AckMessage) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if ack.RuleID == "" {
		ack.RuleID = e.ruleID
	}
	if ack.FromNodeID == "" {
		ack.FromNodeID = e.nodeID
	}
	if ack.AckedAt.IsZero() {
		ack.AckedAt = time.Now()
	}
	if ack.RecordID == "" {
		return fmt.Errorf("ack record_id is required")
	}
	if err := bus.PublishRuleAck(ack.RuleID, ack); err != nil {
		return err
	}
	if e.dispatcher != nil {
		e.dispatcher.OnPluginAck(e.ruleID, e.nodeID, ack.RecordID)
	}
	return nil
}

func (e *EngineRPCServer) GetState(key string) ([]byte, error) {
	if err := e.ensureAuthorized(); err != nil {
		return nil, err
	}
	if e.stateMgr == nil || e.stateMgr.GetBackend() == nil {
		return nil, fmt.Errorf("state manager not available")
	}
	return e.stateMgr.GetBackend().Get(context.Background(), key)
}

func (e *EngineRPCServer) SetState(key string, value []byte) error {
	return e.SetStateWithTTL(key, value, 0)
}

func (e *EngineRPCServer) SetStateWithTTL(key string, value []byte, ttl time.Duration) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.stateMgr == nil || e.stateMgr.GetBackend() == nil {
		return fmt.Errorf("state manager not available")
	}
	return e.stateMgr.GetBackend().SetWithTTL(context.Background(), key, value, ttl)
}

func (e *EngineRPCServer) DeleteState(key string) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.stateMgr == nil || e.stateMgr.GetBackend() == nil {
		return fmt.Errorf("state manager not available")
	}
	return e.stateMgr.GetBackend().Delete(context.Background(), key)
}

// RPC 方法：用于 net/rpc 调用
// EmitRPC 是 RPC 版本的 Emit
// 使用 JSON 序列化以支持 map[string]interface{} 类型（gob 无法编码接口类型）
func (e *EngineRPCServer) EmitRPC(args struct {
	EventJSON []byte // Event 的 JSON 序列化字节数组
	ToNodeIDs []string
}, reply *struct{}) error {
	// 反序列化 Event
	var event sdk.Event
	if err := json.Unmarshal(args.EventJSON, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}
	// 如果 ToNodeIDs 为空，使用保存的 e.toNodeIDs
	// 这样 EngineRPCClient.Emit 调用时（ToNodeIDs 为 nil）可以使用默认的下游节点列表
	toNodeIDs := args.ToNodeIDs
	if len(toNodeIDs) == 0 {
		toNodeIDs = e.toNodeIDs
	}
	return e.EmitWithTargets(event, toNodeIDs)
}

// LogRPC 是 RPC 版本的 Log
func (e *EngineRPCServer) LogRPC(args struct {
	Level  sdk.LogLevel
	Msg    string
	Fields map[string]interface{}
}, reply *struct{}) error {
	e.Log(args.Level, args.Msg, args.Fields)
	return nil
}

// IncCounterRPC 是 RPC 版本的 IncCounter
func (e *EngineRPCServer) IncCounterRPC(args *sdk.MetricArgs, reply *struct{}) error {
	e.IncCounter(args.Name, args.Labels)
	return nil
}

// ObserveRPC 是 RPC 版本的 Observe
func (e *EngineRPCServer) ObserveRPC(args *sdk.MetricArgs, reply *struct{}) error {
	e.Observe(args.Name, args.Value, args.Labels)
	return nil
}

// AckRPC 是 RPC 版本的 Ack
func (e *EngineRPCServer) AckRPC(args *sdk.AckArgs, reply *struct{}) error {
	return e.Ack(args.EventID)
}

func (e *EngineRPCServer) PublishAckRPC(args *sdk.PublishAckArgs, reply *struct{}) error {
	if args == nil {
		return fmt.Errorf("ack args is nil")
	}
	return e.PublishAck(args.Ack)
}

// EmitBatchRPC 是 RPC 版本的 EmitBatch
func (e *EngineRPCServer) EmitBatchRPC(args *sdk.EmitBatchArgs, reply *struct{}) error {
	events := make([]sdk.Event, len(args.EventsJSON))
	for i, data := range args.EventsJSON {
		var event sdk.Event
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("failed to unmarshal event %d in batch: %w", i, err)
		}
		events[i] = event
	}
	return e.EmitBatch(events)
}

// LogBatchRPC 是 RPC 版本的 LogBatch
func (e *EngineRPCServer) LogBatchRPC(args *sdk.LogBatchArgs, reply *struct{}) error {
	e.LogBatch(args.Level, args.Messages, args.Fields)
	return nil
}

func (e *EngineRPCServer) ReportHealthRPC(args *sdk.ReportHealthArgs, reply *struct{}) error {
	if args == nil {
		return fmt.Errorf("health args is nil")
	}
	return e.ReportHealth(*args)
}

func (e *EngineRPCServer) GetStateRPC(args *StateKeyArgs, reply *StateGetReply) error {
	if args == nil {
		return fmt.Errorf("state args is nil")
	}
	data, err := e.GetState(args.Key)
	if err != nil {
		return err
	}
	reply.Value = data
	reply.Found = len(data) > 0
	return nil
}

func (e *EngineRPCServer) SetStateRPC(args *StateSetArgs, reply *struct{}) error {
	if args == nil {
		return fmt.Errorf("state args is nil")
	}
	return e.SetState(args.Key, args.Value)
}

func (e *EngineRPCServer) SetStateWithTTLRPC(args *StateSetWithTTLArgs, reply *struct{}) error {
	if args == nil {
		return fmt.Errorf("state args is nil")
	}
	return e.SetStateWithTTL(args.Key, args.Value, time.Duration(args.TTLNanos))
}

func (e *EngineRPCServer) DeleteStateRPC(args *StateKeyArgs, reply *struct{}) error {
	if args == nil {
		return fmt.Errorf("state args is nil")
	}
	return e.DeleteState(args.Key)
}

func (e *EngineRPCServer) ConnectorExecuteRPC(args *sdk.ConnectorRequest, reply *ConnectorExecuteRPCReply) error {
	if args == nil {
		return fmt.Errorf("connector request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.connector == nil {
		reply.Error = "connector bridge is not available"
		return nil
	}

	var (
		resp sdk.ConnectorResponse
		err  error
	)
	switch args.Kind {
	case sdk.ConnectorRequestRead:
		resp, err = e.connector.Read(*args)
	case sdk.ConnectorRequestWrite:
		resp, err = e.connector.Write(*args)
	case sdk.ConnectorRequestBatchRead:
		resp, err = e.connector.BatchRead(*args)
	case sdk.ConnectorRequestBatchWrite:
		resp, err = e.connector.BatchWrite(*args)
	default:
		reply.Error = fmt.Sprintf("unsupported connector request kind: %s", args.Kind)
		return nil
	}
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	reply.Response = resp
	return nil
}

func (e *EngineRPCServer) ConnectorOpenStreamRPC(args *sdk.StreamOpenRequest, reply *ConnectorOpenStreamRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream open request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.connector == nil {
		reply.Error = "connector bridge is not available"
		return nil
	}
	stream, err := e.connector.OpenStream(*args)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	if stream == nil {
		reply.Error = "connector stream is not available"
		return nil
	}
	streamID := stream.ID()
	if streamID == "" {
		reply.Error = "connector stream id is empty"
		return nil
	}
	e.streamMu.Lock()
	e.streams[streamID] = stream
	e.streamMu.Unlock()
	reply.Response = sdk.StreamOpenResponse{
		StreamID:          streamID,
		DeliverySemantics: sdk.StreamDeliveryAtLeastOnce,
		InitialCredits:    args.InitialCredits,
	}
	return nil
}

func (e *EngineRPCServer) ConnectorReceiveStreamRPC(args *sdk.StreamReceiveRequest, reply *ConnectorReceiveStreamRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream receive request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	stream := e.getStream(args.StreamID)
	if stream == nil {
		reply.Error = "connector stream not found"
		return nil
	}
	message, ok, err := stream.Recv(time.Duration(args.WaitTimeoutMS) * time.Millisecond)
	if err != nil {
		reply.Error = err.Error()
		return nil
	}
	if ok {
		reply.Response = sdk.StreamReceiveResponse{Messages: []sdk.StreamMessage{message}}
	}
	return nil
}

func (e *EngineRPCServer) ConnectorAckStreamRPC(args *sdk.StreamAckRequest, reply *ConnectorAckStreamRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream ack request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	stream := e.getStream(args.StreamID)
	if stream == nil {
		reply.Error = "connector stream not found"
		return nil
	}
	if err := stream.Ack(args.DeliveryID, args.Credits); err != nil {
		reply.Error = err.Error()
	}
	return nil
}

func (e *EngineRPCServer) ConnectorNackStreamRPC(args *sdk.StreamNackRequest, reply *ConnectorNackStreamRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream nack request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	stream := e.getStream(args.StreamID)
	if stream == nil {
		reply.Error = "connector stream not found"
		return nil
	}
	if err := stream.Nack(args.DeliveryID, args.Requeue, args.Reason, args.Credits); err != nil {
		reply.Error = err.Error()
	}
	return nil
}

func (e *EngineRPCServer) ConnectorGrantCreditsRPC(args *sdk.StreamGrantCreditsRequest, reply *ConnectorGrantCreditsRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream grant request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	stream := e.getStream(args.StreamID)
	if stream == nil {
		reply.Error = "connector stream not found"
		return nil
	}
	if err := stream.GrantCredits(args.Credits); err != nil {
		reply.Error = err.Error()
	}
	return nil
}

func (e *EngineRPCServer) ConnectorCloseStreamRPC(args *sdk.StreamCloseRequest, reply *ConnectorCloseStreamRPCReply) error {
	if args == nil {
		return fmt.Errorf("stream close request is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	stream := e.takeStream(args.StreamID)
	if stream == nil {
		return nil
	}
	if err := stream.Close(); err != nil {
		reply.Error = err.Error()
	}
	return nil
}

func (e *EngineRPCServer) CurrentResourceStatusRPC(args *CurrentResourceStatusRPCArgs, reply *CurrentResourceStatusRPCReply) error {
	if args == nil {
		return fmt.Errorf("resource status args is nil")
	}
	if err := e.ensureAuthorized(); err != nil {
		return err
	}
	if e.connector == nil {
		reply.Found = false
		return nil
	}
	event, found := e.connector.CurrentStatus(args.ResourceRef)
	reply.Event = event
	reply.Found = found
	return nil
}

func (e *EngineRPCServer) getStream(streamID string) sdk.StreamConsumer {
	e.streamMu.Lock()
	defer e.streamMu.Unlock()
	return e.streams[streamID]
}

func (e *EngineRPCServer) takeStream(streamID string) sdk.StreamConsumer {
	e.streamMu.Lock()
	defer e.streamMu.Unlock()
	stream := e.streams[streamID]
	delete(e.streams, streamID)
	return stream
}

func (e *EngineRPCServer) NextResourceEventRPC(args *NextResourceEventRPCArgs, reply *NextResourceEventRPCReply) error {
	if err := e.ensureAuthorized(); err != nil {
		return err
	}

	timeout := time.Second
	if args != nil && args.TimeoutMS > 0 {
		timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	events := e.events
	select {
	case event, ok := <-events:
		if !ok {
			reply.OK = false
			return nil
		}
		reply.Event = event
		reply.OK = true
		return nil
	case <-timer.C:
		reply.OK = false
		return nil
	}
}
