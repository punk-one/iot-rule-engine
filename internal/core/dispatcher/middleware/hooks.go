package middleware

import (
	"fmt"
	"time"

	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

// HooksMiddleware Hooks 调用中间件
type HooksMiddleware struct {
	hooks Hooks
}

// Hooks Hook 回调接口
type Hooks interface {
	OnNodeReceive(ruleID, nodeID, eventID string)
	OnNodeEmit(ruleID, nodeID, eventID string)
	OnTaskDone(ruleID, nodeID, eventID string, err error)
}

// Name 返回中间件名称
func (m *HooksMiddleware) Name() string {
	return "Hooks"
}

// Process 调用 Hooks
func (m *HooksMiddleware) Process(ctx *EventContext, next NextFunc) error {
	if m.hooks == nil {
		return next()
	}

	// OnNodeEmit: 节点发出事件
	m.hooks.OnNodeEmit(ctx.RuleID, ctx.FromNode, ctx.Event.ID)

	// 执行后续中间件
	err := next()

	// OnTaskDone: 任务完成
	m.hooks.OnTaskDone(ctx.RuleID, ctx.FromNode, ctx.Event.ID, err)

	return err
}

// NewHooksMiddleware 创建 Hooks 中间件
func NewHooksMiddleware(hooks Hooks) *HooksMiddleware {
	return &HooksMiddleware{
		hooks: hooks,
	}
}

// MetricsMiddleware 指标采集中间件
type MetricsMiddleware struct {
	// 可以添加 Prometheus metrics 等
}

// Name 返回中间件名称
func (m *MetricsMiddleware) Name() string {
	return "Metrics"
}

// Process 采集指标
func (m *MetricsMiddleware) Process(ctx *EventContext, next NextFunc) error {
	start := time.Now()

	err := next()

	duration := time.Since(start)

	// 记录指标
	pkglog.Debug("Event %s processed in %v (rule: %s, from: %s)",
		ctx.Event.ID, duration, ctx.RuleID, ctx.FromNode,
	)

	// 存储延迟信息
	ctx.Set("latency_ms", float64(duration.Milliseconds()))

	return err
}

// NewMetricsMiddleware 创建指标中间件
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{}
}

// TracingMiddleware 链路追踪中间件
type TracingMiddleware struct {
}

// Name 返回中间件名称
func (m *TracingMiddleware) Name() string {
	return "Tracing"
}

// Process 注入追踪信息
func (m *TracingMiddleware) Process(ctx *EventContext, next NextFunc) error {
	event := ctx.Event

	if event.Metadata == nil {
		event.Metadata = make(map[string]string)
	}

	if sdk.GetTraceID(*event) == "" {
		err := fmt.Errorf("missing trace_id (rule=%s, from=%s, event=%s)", ctx.RuleID, ctx.FromNode, event.ID)
		ctx.AddError(err)
		ctx.Stop()
		return err
	}

	// Scheduling context is kept outside plugin-controlled Metadata.
	event.RuleID = ctx.RuleID
	event.Source = ctx.FromNode

	return next()
}

// NewTracingMiddleware 创建链路追踪中间件
func NewTracingMiddleware() *TracingMiddleware {
	return &TracingMiddleware{}
}
