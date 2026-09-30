package middleware

import (
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// Pipeline 中间件处理管道
type Pipeline struct {
	middlewares []Middleware
}

// NewPipeline 创建新的处理管道
func NewPipeline(middlewares ...Middleware) *Pipeline {
	return &Pipeline{
		middlewares: middlewares,
	}
}

// Use 添加中间件
func (p *Pipeline) Use(m Middleware) *Pipeline {
	p.middlewares = append(p.middlewares, m)
	return p
}

// UseFunc 添加函数类型中间件
func (p *Pipeline) UseFunc(name string, fn func(ctx *EventContext, next NextFunc) error) *Pipeline {
	p.middlewares = append(p.middlewares, &namedMiddlewareFunc{
		name: name,
		fn:   fn,
	})
	return p
}

// namedMiddlewareFunc 带名称的中间件函数
type namedMiddlewareFunc struct {
	name string
	fn   func(ctx *EventContext, next NextFunc) error
}

func (m *namedMiddlewareFunc) Name() string {
	return m.name
}

func (m *namedMiddlewareFunc) Process(ctx *EventContext, next NextFunc) error {
	return m.fn(ctx, next)
}

// Execute 执行处理管道
func (p *Pipeline) Execute(ctx *EventContext) error {
	if len(p.middlewares) == 0 {
		return nil
	}

	// 构建执行链
	var chain func(index int) error
	chain = func(index int) error {
		if index >= len(p.middlewares) {
			return nil
		}

		m := p.middlewares[index]
		return m.Process(ctx, func() error {
			if ctx.IsStopped() {
				return nil
			}
			return chain(index + 1)
		})
	}

	return chain(0)
}

// ExecuteWithLogging 带日志的执行处理管道
func (p *Pipeline) ExecuteWithLogging(ctx *EventContext) error {
	if len(p.middlewares) == 0 {
		return nil
	}

	var chain func(index int) error
	chain = func(index int) error {
		if index >= len(p.middlewares) {
			return nil
		}

		m := p.middlewares[index]
		pkglog.Debug("Executing middleware: %s (event: %s)", m.Name(), ctx.Event.ID)

		err := m.Process(ctx, func() error {
			if ctx.IsStopped() {
				pkglog.Debug("Pipeline stopped at middleware: %s", m.Name())
				return nil
			}
			return chain(index + 1)
		})

		if err != nil {
			pkglog.Error("Middleware %s failed: %v", m.Name(), err)
		}

		return err
	}

	return chain(0)
}

// Clone 克隆管道（用于并发安全）
func (p *Pipeline) Clone() *Pipeline {
	middlewares := make([]Middleware, len(p.middlewares))
	copy(middlewares, p.middlewares)
	return &Pipeline{
		middlewares: middlewares,
	}
}

// Len 返回中间件数量
func (p *Pipeline) Len() int {
	return len(p.middlewares)
}
