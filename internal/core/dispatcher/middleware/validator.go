package middleware

import (
	"encoding/json"
	"fmt"
)

// ValidatorMiddleware 事件验证中间件
type ValidatorMiddleware struct {
	MaxPayloadKB int // 最大 Payload 大小（KB）
}

// Name 返回中间件名称
func (m *ValidatorMiddleware) Name() string {
	return "Validator"
}

// Process 验证事件
func (m *ValidatorMiddleware) Process(ctx *EventContext, next NextFunc) error {
	event := ctx.Event
	if event == nil {
		ctx.AddError(fmt.Errorf("event is nil"))
		ctx.Stop()
		return fmt.Errorf("event is nil")
	}

	// 验证事件 ID
	if event.ID == "" {
		ctx.AddError(fmt.Errorf("event ID is empty"))
		ctx.Stop()
		return fmt.Errorf("event ID is empty")
	}

	// 验证 Payload 大小
	if m.MaxPayloadKB > 0 {
		payloadBytes, err := json.Marshal(event.Payload)
		if err != nil {
			ctx.AddError(fmt.Errorf("failed to marshal payload: %w", err))
			ctx.Stop()
			return err
		}

		payloadKB := len(payloadBytes) / 1024
		if payloadKB > m.MaxPayloadKB {
			err := fmt.Errorf("payload size %dKB exceeds limit %dKB", payloadKB, m.MaxPayloadKB)
			ctx.AddError(err)
			ctx.Stop()
			return err
		}
	}

	// 验证来源节点
	if ctx.FromNode == "" {
		ctx.AddError(fmt.Errorf("from node is empty"))
		ctx.Stop()
		return fmt.Errorf("from node is empty")
	}

	return next()
}

// NewValidatorMiddleware 创建验证中间件
func NewValidatorMiddleware(maxPayloadKB int) *ValidatorMiddleware {
	return &ValidatorMiddleware{
		MaxPayloadKB: maxPayloadKB,
	}
}
