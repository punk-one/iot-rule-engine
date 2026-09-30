package dispatcher

import (
	"context"
	"fmt"

	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/middleware"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher/scheduler"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// PipelineConfig Pipeline 配置
type PipelineConfig struct {
	EnableValidation   bool
	EnableRateLimiting bool
	EnableDedup        bool
	EnableMetrics      bool
	EnableTracing      bool
	MaxPayloadKB       int
	RuleQPS            int
	NodeQPS            int
	DedupWindowMs      int
}

// DefaultPipelineConfig 默认 Pipeline 配置
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		EnableValidation:   true,
		EnableRateLimiting: true,
		EnableDedup:        true,
		EnableMetrics:      true,
		EnableTracing:      true,
		MaxPayloadKB:       256,
		RuleQPS:            10000,
		NodeQPS:            1000,
		DedupWindowMs:      1000,
	}
}

// DispatcherPipeline Dispatcher 处理管道
type DispatcherPipeline struct {
	pipeline   *middleware.Pipeline
	dispatcher *Dispatcher
	config     PipelineConfig
}

// NewDispatcherPipeline 创建 Dispatcher Pipeline
func NewDispatcherPipeline(d *Dispatcher, cfg PipelineConfig) *DispatcherPipeline {
	dp := &DispatcherPipeline{
		pipeline:   middleware.NewPipeline(),
		dispatcher: d,
		config:     cfg,
	}

	// 按顺序添加中间件
	dp.setupMiddlewares()

	return dp
}

// setupMiddlewares 设置中间件链
func (dp *DispatcherPipeline) setupMiddlewares() {
	// 1. Tracing 中间件（最先执行，确保所有事件都有 trace_id）
	if dp.config.EnableTracing {
		dp.pipeline.Use(middleware.NewTracingMiddleware())
	}

	// 2. Metrics 中间件（记录处理延迟）
	if dp.config.EnableMetrics {
		dp.pipeline.Use(middleware.NewMetricsMiddleware())
	}

	// 3. Validation 中间件（验证事件格式和大小）
	if dp.config.EnableValidation {
		dp.pipeline.Use(middleware.NewValidatorMiddleware(dp.config.MaxPayloadKB))
	}

	// 4. Rate Limiting 中间件（限流）
	if dp.config.EnableRateLimiting {
		dp.pipeline.Use(middleware.NewLimiterMiddleware(dp.config.RuleQPS, dp.config.NodeQPS))
	}

	// 5. Dedup 中间件（去重）
	if dp.config.EnableDedup {
		dp.pipeline.Use(middleware.NewDedupMiddleware(dp.config.DedupWindowMs))
	}

	// 6. Hooks 中间件（通知 Observer）
	dp.pipeline.Use(&hooksMiddlewareAdapter{dispatcher: dp.dispatcher})

	// 7. Router 中间件（解析路由）
	dp.pipeline.Use(&routerMiddleware{dispatcher: dp.dispatcher})

	// 8. Publisher 中间件（发布到 Scheduler）
	dp.pipeline.Use(&publisherMiddleware{dispatcher: dp.dispatcher})
}

// ProcessEvent 通过 Pipeline 处理事件
func (dp *DispatcherPipeline) ProcessEvent(ruleID, fromNodeID string, event *sdk.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	owned := event.Clone()
	owned.RuleID = ruleID
	owned.Source = fromNodeID
	ctx := middleware.NewEventContext(context.Background(), ruleID, fromNodeID, &owned)

	err := dp.pipeline.ExecuteWithLogging(ctx)

	// 检查是否有错误
	if ctx.HasErrors() {
		for _, e := range ctx.Errors {
			pkglog.Error("Pipeline error: %v", e)
		}
	}

	return err
}

// hooksMiddlewareAdapter 适配 Dispatcher Hooks 到中间件
type hooksMiddlewareAdapter struct {
	dispatcher *Dispatcher
}

func (m *hooksMiddlewareAdapter) Name() string {
	return "DispatcherHooks"
}

func (m *hooksMiddlewareAdapter) Process(ctx *middleware.EventContext, next middleware.NextFunc) error {
	// 通知 Observer 更新 EventsOut
	if m.dispatcher.hooks.OnNodeEmit != nil {
		m.dispatcher.notifyNodeEmit(ctx.RuleID, ctx.FromNode, *ctx.Event)
	}

	return next()
}

// routerMiddleware 路由解析中间件
type routerMiddleware struct {
	dispatcher *Dispatcher
}

func (m *routerMiddleware) Name() string {
	return "Router"
}

func (m *routerMiddleware) Process(ctx *middleware.EventContext, next middleware.NextFunc) error {
	// 1. 尝试使用 Logical Router 解析路由
	if targets := m.dispatcher.router.Resolve(*ctx.Event); len(targets) > 0 {
		ctx.SetToNodes(targets)
		pkglog.Debug("Resolved logical route from %s to %v", ctx.FromNode, targets)
		return next()
	}

	// 2. 回退到物理拓扑路由
	m.dispatcher.rulesMu.RLock()
	rule, exists := m.dispatcher.rules[ctx.RuleID]
	m.dispatcher.rulesMu.RUnlock()

	if !exists {
		ctx.AddError(fmt.Errorf("rule %s not found", ctx.RuleID))
		ctx.Stop()
		return fmt.Errorf("rule %s not found", ctx.RuleID)
	}

	toNodeIDs, ok := rule.Edges[ctx.FromNode]
	if !ok || len(toNodeIDs) == 0 {
		pkglog.Debug("No downstream nodes for node %s in rule %s", ctx.FromNode, ctx.RuleID)
		return nil // 没有下游节点是正常的（Sink 节点）
	}

	ctx.SetToNodes(toNodeIDs)
	return next()
}

// publisherMiddleware 事件发布中间件
type publisherMiddleware struct {
	dispatcher *Dispatcher
}

func (m *publisherMiddleware) Name() string {
	return "Publisher"
}

func (m *publisherMiddleware) Process(ctx *middleware.EventContext, next middleware.NextFunc) error {
	if len(ctx.ToNodes) == 0 {
		return nil // 没有目标节点
	}

	for _, toNodeID := range ctx.ToNodes {
		var reservation *DedupReservation
		if m.dispatcher.deduplicator != nil {
			var duplicate bool
			reservation, duplicate = m.dispatcher.deduplicator.Reserve(ctx.RuleID, ctx.FromNode, toNodeID, ctx.Event.ID)
			if duplicate {
				pkglog.Debug("Event deduplicated by publisher (rule=%s, from=%s, to=%s, event=%s)",
					ctx.RuleID, ctx.FromNode, toNodeID, ctx.Event.ID)
				continue
			}
		}

		// Every downstream branch owns an independent Event.
		branchEvent := ctx.Event.Clone()
		task := &scheduler.DispatchTask{
			RuleID:   ctx.RuleID,
			FromNode: ctx.FromNode,
			ToNode:   toNodeID,
			Event:    branchEvent,
			TraceCtx: ctx.Context,
		}

		if err := m.dispatcher.scheduler.Submit(task); err != nil {
			reservation.Rollback()
			pkglog.Error("Failed to submit task (From: %s, To: %s): %v", ctx.FromNode, toNodeID, err)
			if m.dispatcher.errorHandler != nil {
				m.dispatcher.errorHandler(toNodeID, ctx.Event.ID, err)
			}
			ctx.AddError(err)
		} else {
			reservation.Commit()
		}
	}

	return next()
}

// UsePipeline 使用 Pipeline 处理事件（替代 OnPluginEmit 内部逻辑）
// 这个方法可以用来逐步迁移到 Pipeline 模式
func (d *Dispatcher) UsePipeline(cfg PipelineConfig) *DispatcherPipeline {
	return NewDispatcherPipeline(d, cfg)
}

// OnPluginEmitWithPipeline 使用 Pipeline 处理插件发出的事件
// 这是 OnPluginEmit 的 Pipeline 版本
func (d *Dispatcher) OnPluginEmitWithPipeline(pipeline *DispatcherPipeline, ruleID, fromNodeID string, event *sdk.Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	return pipeline.ProcessEvent(ruleID, fromNodeID, event)
}
