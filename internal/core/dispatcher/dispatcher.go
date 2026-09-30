package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/client"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/limiter"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/router"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/utils"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// Dispatcher 是当前核心调度器
// 职责：订阅 NATS Edge Topic、解析 Event、通过 Router 路由并提交给 Scheduler 执行
// 通过 Scheduler 接口解耦调度逻辑
type Dispatcher struct {
	ruleID RuleID // 当前规则ID

	js               *bus.JetStreamClient
	edgeSubscription *bus.SubscriptionHandle
	mu               sync.RWMutex // 保护 started 字段

	// 多规则运行时管理
	rules                map[string]*RuleRuntime        // ruleID -> RuleRuntime
	startupAuthorizedRPC map[string]map[string]struct{} // ruleID -> nodeID set
	rulesMu              sync.RWMutex

	ctx     context.Context
	cancel  context.CancelFunc
	started bool

	// 调度器
	scheduler scheduler.Scheduler
	router    *router.Router // 逻辑路由组件

	// 安全组件
	rateLimiter  *limiter.DispatcherLimiter // Rule/Node 级限流器
	deduplicator *Deduplicator              // 防循环去重器
	maxPayloadKB int                        // Payload 最大大小（KB）
	// 背压溢出策略（reject | drop_oldest | block）
	overflowStrategy string

	// Legacy configs（保留签名兼容，主要由 scheduler 接管）
	eventTimeout   time.Duration
	serialDelivery bool
	errorHandler   func(nodeID, eventID string, err error)
	deliveryMode   string

	// Hooks for runtime observation（replaces callbacks）
	// Dispatcher 通过 Hooks 通知 Observer，而不是直接操作 Runtime
	hooks           DispatcherHooks
	publishRuleEdge func(ruleID, fromNodeID, toNodeID string, payload []byte) error
}

// NewDispatcher 创建新的 Dispatcher
func NewDispatcher(ruleID string) *Dispatcher {
	return NewDispatcherWithConfig(ruleID, 5*time.Second, true, "best_effort", nil)
}

// NewDispatcherWithConfig 创建带配置的 Dispatcher
func NewDispatcherWithConfig(ruleID string, eventTimeout time.Duration, serialDelivery bool, deliveryMode string, errorHandler func(nodeID, eventID string, err error)) *Dispatcher {
	return NewDispatcherWithSecurityConfig(ruleID, eventTimeout, serialDelivery, deliveryMode, errorHandler, 100000, 10000, 1000, 1000, 256, "reject")
}

// SetHooks 设置 Dispatcher Hooks
// 由 Executor 调用，注入 RuntimeObserver
func (d *Dispatcher) SetHooks(hooks DispatcherHooks) {
	d.hooks = hooks
}

// NewDispatcherWithSecurityConfig 创建带安全配置的 Dispatcher
func NewDispatcherWithSecurityConfig(
	ruleID string,
	eventTimeout time.Duration,
	serialDelivery bool,
	deliveryMode string,
	errorHandler func(nodeID, eventID string, err error),
	maxQueue int,
	ruleQPS int,
	nodeQPS int,
	dedupWindowMs int,
	maxPayloadKB int,
	overflowStrategy string,
) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())

	if eventTimeout <= 0 {
		eventTimeout = 5 * time.Second
	}
	if deliveryMode == "" {
		deliveryMode = "best_effort"
	}
	if maxQueue <= 0 {
		maxQueue = 100000 // 默认队列大小
	}
	if ruleQPS <= 0 {
		ruleQPS = 10000 // 默认 Rule QPS
	}
	if nodeQPS <= 0 {
		nodeQPS = 1000 // 默认 Node QPS
	}
	if dedupWindowMs <= 0 {
		dedupWindowMs = 1000 // 默认1秒
	}
	if maxPayloadKB <= 0 {
		maxPayloadKB = 256 // 默认256KB
	}
	if overflowStrategy == "" {
		overflowStrategy = "reject"
	}

	d := &Dispatcher{
		ruleID:               RuleID(ruleID),
		rules:                make(map[string]*RuleRuntime),
		startupAuthorizedRPC: make(map[string]map[string]struct{}),
		ctx:                  ctx,
		cancel:               cancel,
		started:              false,
		eventTimeout:         eventTimeout,
		serialDelivery:       serialDelivery,
		errorHandler:         errorHandler,
		deliveryMode:         deliveryMode,
		maxPayloadKB:         maxPayloadKB,
		overflowStrategy:     overflowStrategy,
		hooks:                DispatcherHooks{}, // 由 Executor 注入
		publishRuleEdge:      bus.PublishRuleEdge,
	}

	// 初始化默认 FIFO Scheduler
	// 使用 closure capture 'd' 作为 executor
	executor := func(task *scheduler.DispatchTask) error {
		return d.executeTask(task)
	}
	d.scheduler = scheduler.NewFifoScheduler(maxQueue, overflowStrategy, executor)
	d.router = router.NewRouter() // 初始化 Router

	// 初始化安全组件
	d.rateLimiter = limiter.NewDispatcherLimiter(limiter.LimiterConfig{
		RuleQPS: ruleQPS,
		NodeQPS: nodeQPS,
		Burst:   ruleQPS, // 默认 burst = RuleQPS
	})
	d.deduplicator = NewDeduplicator(time.Duration(dedupWindowMs) * time.Millisecond)

	return d
}

// RegisterRouter 注册逻辑路由规则
func (d *Dispatcher) RegisterRouter(nodeID string, edges map[string]string) {
	d.router.RegisterRouter(nodeID, edges)
}

// OnPluginEmit 处理插件发出的事件
// 查找下游并提交给 Scheduler
func (d *Dispatcher) OnPluginEmit(ruleID, fromNodeID string, event *sdk.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	// Take ownership at the dispatcher boundary. The caller may reuse or mutate
	// its event immediately after this method returns.
	baseEvent := event.Clone()

	// Payload 大小检查
	if err := d.checkPayloadSize(&baseEvent); err != nil {
		if d.errorHandler != nil {
			d.errorHandler(fromNodeID, baseEvent.ID, err)
		}
		return err
	}

	// Rule/Node 级限流
	if d.rateLimiter != nil && !d.rateLimiter.Allow(ruleID, fromNodeID) {
		err := ErrRateLimited
		pkglog.Warn("Dispatcher rate limited (rule=%s, node=%s, event=%s)", ruleID, fromNodeID, baseEvent.ID)
		if d.errorHandler != nil {
			d.errorHandler(fromNodeID, baseEvent.ID, err)
		}
		return err
	}

	if err := enrichTrace(&baseEvent, ruleID, fromNodeID); err != nil {
		if d.errorHandler != nil {
			d.errorHandler(fromNodeID, baseEvent.ID, err)
		}
		return err
	}

	toNodeIDs, err := d.resolveToNodeIDs(ruleID, fromNodeID, &baseEvent)
	if err != nil {
		return err
	}
	if len(toNodeIDs) == 0 {
		// 即使没有下游节点，也要通知 Observer 更新 EventsOut
		if d.hooks.OnNodeEmit != nil {
			d.notifyNodeEmit(ruleID, fromNodeID, baseEvent)
		}
		return nil
	}

	// 通知 Observer 更新 EventsOut（通过 Hooks）
	if d.hooks.OnNodeEmit != nil {
		d.notifyNodeEmit(ruleID, fromNodeID, baseEvent)
	}

	// 提交任务给 Scheduler
	var firstErr error
	for _, toNodeID := range toNodeIDs {
		var reservation *DedupReservation
		if d.deduplicator != nil {
			var duplicate bool
			reservation, duplicate = d.deduplicator.Reserve(ruleID, fromNodeID, toNodeID, baseEvent.ID)
			if duplicate {
				pkglog.Debug("Event deduplicated (rule=%s, from=%s, to=%s, event=%s)", ruleID, fromNodeID, toNodeID, baseEvent.ID)
				continue
			}
		}

		// Every downstream branch owns an independent Event.
		branchEvent := baseEvent.Clone()
		task := &scheduler.DispatchTask{
			RuleID:   ruleID,
			FromNode: fromNodeID,
			ToNode:   toNodeID,
			Event:    branchEvent,
			TraceCtx: context.Background(),
		}

		if err := d.scheduler.Submit(task); err != nil {
			reservation.Rollback()
			wrappedErr := err
			if err == scheduler.ErrQueueFull {
				bpErr := &BackpressureError{
					RuleID:    ruleID,
					FromNode:  fromNodeID,
					ToNode:    toNodeID,
					Strategy:  d.overflowStrategy,
					QueueHint: "scheduler queue full",
				}
				wrappedErr = bpErr
				pkglog.Warn("Scheduler queue full (From: %s, To: %s, event=%s, strategy=%s)", fromNodeID, toNodeID, baseEvent.ID, d.overflowStrategy)
				if d.hooks.OnNodeMetric != nil {
					d.hooks.OnNodeMetric(ruleID, fromNodeID, "dispatcher.backpressure", 1, map[string]string{
						"to_node":  toNodeID,
						"strategy": d.overflowStrategy,
					})
				}
			}
			if d.errorHandler != nil {
				d.errorHandler(toNodeID, baseEvent.ID, wrappedErr)
			}
			pkglog.Error("Failed to submit task (From: %s, To: %s): %v", fromNodeID, toNodeID, wrappedErr)
			if firstErr == nil {
				firstErr = wrappedErr
			}
		} else {
			reservation.Commit()
		}
	}

	return firstErr
}

func (d *Dispatcher) resolveToNodeIDs(ruleID, fromNodeID string, event *sdk.Event) ([]string, error) {
	// 1. 尝试使用 Logical Router 解析路由
	if targets := d.router.Resolve(*event); len(targets) > 0 {
		pkglog.Debug("Resolved logical route from %s via label %s to %v", fromNodeID, event.Metadata["route_label"], targets)
		return targets, nil
	}

	// 2. 回退到物理拓扑路由
	d.rulesMu.RLock()
	rule, exists := d.rules[ruleID]
	d.rulesMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("rule %s not found", ruleID)
	}

	toNodeIDs, ok := rule.Edges[fromNodeID]
	if !ok || len(toNodeIDs) == 0 {
		pkglog.Debug("No downstream nodes for node %s in rule %s", fromNodeID, ruleID)
		return nil, nil
	}

	return toNodeIDs, nil
}

// checkPayloadSize 检查 Payload 大小
func (d *Dispatcher) checkPayloadSize(event *sdk.Event) error {
	if d.maxPayloadKB <= 0 {
		return nil // 未配置，不检查
	}

	// 编码事件以获取实际大小
	data, err := EncodeEvent(*event)
	if err != nil {
		return fmt.Errorf("failed to encode event for size check: %w", err)
	}

	sizeKB := len(data) / 1024
	if sizeKB > d.maxPayloadKB {
		return fmt.Errorf("%w: event size %dKB exceeds limit %dKB", ErrPayloadTooLarge, sizeKB, d.maxPayloadKB)
	}

	return nil
}

// IsPluginRegistered 检查插件是否已注册（用于 RPC 鉴权）
func (d *Dispatcher) IsPluginRegistered(ruleID, nodeID string) bool {
	// 检查多规则映射
	d.rulesMu.RLock()
	defer d.rulesMu.RUnlock()
	if rule, exists := d.rules[ruleID]; exists {
		if _, ok := rule.Plugins[nodeID]; ok {
			return true
		}
	}
	if nodes, exists := d.startupAuthorizedRPC[ruleID]; exists {
		if _, ok := nodes[nodeID]; ok {
			return true
		}
	}
	return false
}

// OnPluginMetric 处理插件报告的指标
func (d *Dispatcher) OnPluginMetric(ruleID, nodeID string, name string, value float64, labels map[string]string) {
	if d.hooks.OnNodeMetric != nil {
		d.hooks.OnNodeMetric(ruleID, nodeID, name, value, labels)
	}
}

// OnPluginAck 处理插件报告的确认
func (d *Dispatcher) OnPluginAck(ruleID, nodeID string, eventID string) {
	if d.hooks.OnNodeAck != nil {
		d.hooks.OnNodeAck(ruleID, nodeID, eventID)
	}
}

// OnPluginEmitBatch 处理插件批量发出的事件
func (d *Dispatcher) OnPluginEmitBatch(ruleID, fromNodeID string, events []sdk.Event) error {
	if len(events) == 0 {
		return nil
	}

	batchedTasks := make(map[string][]sdk.Event)
	batchReservations := make(map[string][]*DedupReservation)
	localReservations := make(map[string]struct{})
	var failed int
	var firstErr error

	for _, event := range events {
		evt := event.Clone()

		// Payload 大小检查
		if err := d.checkPayloadSize(&evt); err != nil {
			if d.errorHandler != nil {
				d.errorHandler(fromNodeID, evt.ID, err)
			}
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// Rule/Node 级限流
		if d.rateLimiter != nil && !d.rateLimiter.Allow(ruleID, fromNodeID) {
			err := ErrRateLimited
			pkglog.Warn("Dispatcher rate limited (rule=%s, node=%s, event=%s)", ruleID, fromNodeID, evt.ID)
			if d.errorHandler != nil {
				d.errorHandler(fromNodeID, evt.ID, err)
			}
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		if err := enrichTrace(&evt, ruleID, fromNodeID); err != nil {
			pkglog.Warn("Dispatcher rejected batch event without trace (rule=%s, node=%s, event=%s): %v", ruleID, fromNodeID, evt.ID, err)
			if d.errorHandler != nil {
				d.errorHandler(fromNodeID, evt.ID, err)
			}
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		toNodeIDs, err := d.resolveToNodeIDs(ruleID, fromNodeID, &evt)
		if err != nil {
			pkglog.Error("Failed to resolve targets for batch event: %v", err)
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// 即使没有下游节点，也要更新 EventsOut
		if d.hooks.OnNodeEmit != nil {
			d.notifyNodeEmit(ruleID, fromNodeID, evt)
		}
		if len(toNodeIDs) == 0 {
			continue
		}

		for _, toNodeID := range toNodeIDs {
			localKey := toNodeID + "\x00" + evt.ID
			if _, duplicate := localReservations[localKey]; duplicate {
				continue
			}
			localReservations[localKey] = struct{}{}

			var reservation *DedupReservation
			if d.deduplicator != nil {
				var duplicate bool
				reservation, duplicate = d.deduplicator.Reserve(ruleID, fromNodeID, toNodeID, evt.ID)
				if duplicate {
					pkglog.Debug("Event deduplicated in batch (rule=%s, from=%s, to=%s, event=%s)", ruleID, fromNodeID, toNodeID, evt.ID)
					continue
				}
			}
			// Each target batch owns its own clone of every event.
			batchedTasks[toNodeID] = append(batchedTasks[toNodeID], evt.Clone())
			batchReservations[toNodeID] = append(batchReservations[toNodeID], reservation)
		}
	}

	// 按下游节点提交批任务
	for toNodeID, grouped := range batchedTasks {
		task := &scheduler.DispatchTask{
			RuleID:   ruleID,
			FromNode: fromNodeID,
			ToNode:   toNodeID,
			Events:   grouped,
			TraceCtx: context.Background(),
		}

		if err := d.scheduler.Submit(task); err != nil {
			for _, reservation := range batchReservations[toNodeID] {
				reservation.Rollback()
			}
			wrappedErr := err
			if err == scheduler.ErrQueueFull {
				wrappedErr = &BackpressureError{
					RuleID:    ruleID,
					FromNode:  fromNodeID,
					ToNode:    toNodeID,
					Strategy:  d.overflowStrategy,
					QueueHint: "scheduler queue full",
				}
				if d.hooks.OnNodeMetric != nil {
					d.hooks.OnNodeMetric(ruleID, fromNodeID, "dispatcher.backpressure", float64(len(grouped)), map[string]string{
						"to_node":  toNodeID,
						"strategy": d.overflowStrategy,
					})
				}
			}

			for _, evt := range grouped {
				if d.errorHandler != nil {
					d.errorHandler(toNodeID, evt.ID, wrappedErr)
				}
			}

			failed += len(grouped)
			if firstErr == nil {
				firstErr = wrappedErr
			}
		} else {
			for _, reservation := range batchReservations[toNodeID] {
				reservation.Commit()
			}
		}
	}

	if failed > 0 {
		return fmt.Errorf("batch emit failed for %d/%d events: %w", failed, len(events), firstErr)
	}
	return nil
}

// Start 启动 Dispatcher
func (d *Dispatcher) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return fmt.Errorf("dispatcher already started")
	}

	// 启动 Scheduler
	if err := d.scheduler.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// 获取 JetStream 客户端
	js := bus.GetJetStream()
	if js == nil {
		d.scheduler.Stop()
		return fmt.Errorf("JetStream not initialized")
	}
	d.js = js

	// 1. 订阅 Edge Topic（用于 DAG 内部路由）
	// 使用语义级 API，禁止裸 subject
	edgeQueueGroup := fmt.Sprintf("dispatcher-edge-%s", d.ruleID)

	edgeSubscription, err := bus.SubscribeRuleEdgeManualAckManaged(string(d.ruleID), edgeQueueGroup, func(msg *nats.Msg) {
		// 处理 Edge Topic 消息
		if err := d.onMessage(msg.Subject, msg.Data); err != nil {
			pkglog.Error("Handler error for subject %s: %v", msg.Subject, err)
			msg.Nak()
		} else {
			msg.Ack()
		}
	})

	if err != nil {
		d.scheduler.Stop()
		return fmt.Errorf("failed to subscribe to rule edge for rule %s: %w", d.ruleID, err)
	}
	d.edgeSubscription = edgeSubscription

	d.started = true
	pkglog.Info("Dispatcher started for rule %s, subscribed to rule.edge.%s.>", d.ruleID, d.ruleID)
	return nil
}

// Stop 停止 Dispatcher
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.started {
		return
	}
	if d.edgeSubscription != nil {
		if err := d.edgeSubscription.Close(); err != nil {
			pkglog.Warn("Failed to close dispatcher edge subscription for rule %s: %v", d.ruleID, err)
		}
		d.edgeSubscription = nil
	}

	d.cancel()

	// 停止 Scheduler
	if err := d.scheduler.Stop(); err != nil {
		pkglog.Error("Failed to stop scheduler: %v", err)
	}

	d.started = false
	pkglog.Info("Dispatcher stopped for rule %s", d.ruleID)
}

// onMessage 处理 NATS 消息
func (d *Dispatcher) onMessage(subject string, data []byte) error {
	select {
	case <-d.ctx.Done():
		return nil
	default:
	}

	toNodeID, err := d.extractToNodeID(subject)
	if err != nil {
		return err
	}

	event, err := DecodeEvent(data)
	if err != nil {
		return err
	}
	event.RuleID = string(d.ruleID)
	event.Source = "legacy_nats"

	// 提交给 Scheduler
	task := &scheduler.DispatchTask{
		RuleID:   string(d.ruleID),
		FromNode: "legacy_nats",
		ToNode:   toNodeID,
		Event:    event,
		TraceCtx: context.Background(),
	}

	return d.scheduler.Submit(task)
}

// extractToNodeID 从 subject 中提取 ToNodeID
// 使用语义级 API 提取边信息
// 格式：rule.edge.{ruleID}.{fromNodeID}.{toNodeID}
func (d *Dispatcher) extractToNodeID(subject string) (string, error) {
	ruleID, _, toNodeID, err := bus.ExtractEdgeInfo(subject)
	if err != nil {
		return "", err
	}

	// 验证 ruleID 是否匹配
	if ruleID != string(d.ruleID) {
		return "", fmt.Errorf("ruleID mismatch: expected %s, got %s", d.ruleID, ruleID)
	}

	return toNodeID, nil
}

// RegisterRule 注册规则运行时
func (d *Dispatcher) RegisterRule(ruleID string, edges map[string][]string) {
	d.rulesMu.Lock()
	defer d.rulesMu.Unlock()

	d.rules[ruleID] = &RuleRuntime{
		RuleID:  ruleID,
		Plugins: make(map[string]client.PluginClient),
		Edges:   edges,
	}
	pkglog.Info("Dispatcher registered rule: %s", ruleID)
}

// UnregisterRule 注销规则运行时
func (d *Dispatcher) UnregisterRule(ruleID string) {
	d.rulesMu.Lock()
	defer d.rulesMu.Unlock()

	delete(d.rules, ruleID)
	delete(d.startupAuthorizedRPC, ruleID)
	pkglog.Info("Dispatcher unregistered rule: %s", ruleID)
}

// AuthorizePluginStartupRPC 允许插件在正式注册到 RuleRuntime 前访问启动期必须的 EngineRPC。
func (d *Dispatcher) AuthorizePluginStartupRPC(ruleID, nodeID string) {
	d.rulesMu.Lock()
	defer d.rulesMu.Unlock()

	nodes, exists := d.startupAuthorizedRPC[ruleID]
	if !exists {
		nodes = make(map[string]struct{})
		d.startupAuthorizedRPC[ruleID] = nodes
	}
	nodes[nodeID] = struct{}{}
}

// RegisterPluginToRule 将插件注册到指定规则
func (d *Dispatcher) RegisterPluginToRule(ruleID, nodeID string, pClient client.PluginClient) {
	d.rulesMu.Lock()
	defer d.rulesMu.Unlock()

	rule, exists := d.rules[ruleID]
	if !exists {
		// 如果规则不存在，创建一个新的 RuleRuntime
		rule = &RuleRuntime{
			RuleID:  ruleID,
			Plugins: make(map[string]client.PluginClient),
			Edges:   make(map[string][]string),
		}
		d.rules[ruleID] = rule
	}

	rule.Plugins[nodeID] = pClient
	if nodes, exists := d.startupAuthorizedRPC[ruleID]; exists {
		delete(nodes, nodeID)
		if len(nodes) == 0 {
			delete(d.startupAuthorizedRPC, ruleID)
		}
	}
	pkglog.Debug("Dispatcher registered plugin %s to rule %s (type: %s)", nodeID, ruleID, pClient.Info().Type)
}

// OnPluginEmit 处理插件发出的事件
// enrichTrace 为事件注入 Trace 信息
func enrichTrace(event *sdk.Event, ruleID, nodeID string) error {
	if event == nil {
		return fmt.Errorf("%w (rule=%s, node=%s, event=<nil>)", ErrMissingTraceID, ruleID, nodeID)
	}

	utils.EnsureEventTraceID(event)

	// Scheduling context belongs to dedicated fields, never plugin metadata.
	event.RuleID = ruleID
	event.Source = nodeID
	return nil
}

// PublishEventToNodes 发布事件到指定的目标节点列表（供 EngineRPC.EmitEvent 调用）
// 注意：推荐使用 OnPluginEmit 方法
func (d *Dispatcher) PublishEventToNodes(fromNodeID string, toNodeIDs []string, event sdk.Event) error {
	d.mu.RLock()
	started := d.started
	d.mu.RUnlock()
	if !started {
		return ErrDispatcherNotStarted
	}

	if len(toNodeIDs) == 0 {
		// 没有下游节点（Sink 节点），直接返回
		return nil
	}

	// 编码事件
	data, err := EncodeEvent(event)
	if err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	// 发布到所有下游节点的 Edge Topic
	// 使用语义级 API，禁止裸 subject
	publisher := d.publishRuleEdge
	if publisher == nil {
		publisher = bus.PublishRuleEdge
	}
	var publishErrors []error
	for _, toNodeID := range toNodeIDs {
		if err := publisher(string(d.ruleID), fromNodeID, toNodeID, data); err != nil {
			pkglog.Error("Failed to publish event to rule edge (rule=%s, from=%s, to=%s): %v", d.ruleID, fromNodeID, toNodeID, err)
			publishErrors = append(publishErrors, fmt.Errorf("publish edge %s->%s: %w", fromNodeID, toNodeID, err))
			continue
		}

		pkglog.Debug("Dispatcher published event to rule edge (rule=%s, from=%s, to=%s, eventID=%s)", d.ruleID, fromNodeID, toNodeID, event.ID)
	}

	return errors.Join(publishErrors...)
}
