// Package middleware 提供 Dispatcher 事件处理中间件
// Dispatcher 事件处理中间件
package middleware

import (
	"context"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

// EventContext 事件处理上下文
type EventContext struct {
	Context  context.Context
	RuleID   string
	FromNode string
	ToNodes  []string
	Event    *sdk.Event
	Metadata map[string]interface{}
	Errors   []error
	Stopped  bool
}

// NewEventContext 创建事件上下文
func NewEventContext(ctx context.Context, ruleID, fromNode string, event *sdk.Event) *EventContext {
	return &EventContext{
		Context:  ctx,
		RuleID:   ruleID,
		FromNode: fromNode,
		Event:    event,
		Metadata: make(map[string]interface{}),
		Errors:   []error{},
		Stopped:  false,
	}
}

// SetToNodes 设置目标节点
func (c *EventContext) SetToNodes(nodes []string) {
	c.ToNodes = nodes
}

// AddError 添加错误
func (c *EventContext) AddError(err error) {
	c.Errors = append(c.Errors, err)
}

// HasErrors 检查是否有错误
func (c *EventContext) HasErrors() bool {
	return len(c.Errors) > 0
}

// Stop 停止处理链
func (c *EventContext) Stop() {
	c.Stopped = true
}

// IsStopped 检查是否已停止
func (c *EventContext) IsStopped() bool {
	return c.Stopped
}

// Set 设置元数据
func (c *EventContext) Set(key string, value interface{}) {
	c.Metadata[key] = value
}

// Get 获取元数据
func (c *EventContext) Get(key string) (interface{}, bool) {
	v, ok := c.Metadata[key]
	return v, ok
}

// GetString 获取字符串类型元数据
func (c *EventContext) GetString(key string) string {
	if v, ok := c.Metadata[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// NextFunc 下一个中间件函数类型
type NextFunc func() error

// Middleware 中间件接口
type Middleware interface {
	// Name 返回中间件名称（用于日志和调试）
	Name() string
	// Process 处理事件
	// ctx: 事件上下文
	// next: 调用下一个中间件
	Process(ctx *EventContext, next NextFunc) error
}

// MiddlewareFunc 中间件函数类型适配器
type MiddlewareFunc func(ctx *EventContext, next NextFunc) error

// Process 实现 Middleware 接口
func (f MiddlewareFunc) Process(ctx *EventContext, next NextFunc) error {
	return f(ctx, next)
}

// Name 返回中间件名称
func (f MiddlewareFunc) Name() string {
	return "MiddlewareFunc"
}
