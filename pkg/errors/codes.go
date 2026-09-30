// Package errors 提供统一的错误处理框架
// v1.5.0: 重构建议 - 统一错误码体系
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode 错误码类型
type ErrorCode string

// 错误码常量定义
const (
	// Rule 相关错误 (RULE_xxx)
	ErrCodeRuleNotFound      ErrorCode = "RULE_001" // 规则不存在
	ErrCodeRuleAlreadyExists ErrorCode = "RULE_002" // 规则已存在
	ErrCodeRuleInvalidFormat ErrorCode = "RULE_003" // 规则格式错误
	ErrCodeRuleValidation    ErrorCode = "RULE_004" // 规则验证失败
	ErrCodeRuleDisabled      ErrorCode = "RULE_005" // 规则已禁用

	// Plugin 相关错误 (PLUGIN_xxx)
	ErrCodePluginNotFound    ErrorCode = "PLUGIN_001" // 插件未找到
	ErrCodePluginNotLoaded   ErrorCode = "PLUGIN_002" // 插件未加载
	ErrCodePluginLoadFailed  ErrorCode = "PLUGIN_003" // 插件加载失败
	ErrCodePluginStartFailed ErrorCode = "PLUGIN_004" // 插件启动失败
	ErrCodePluginRPCTimeout  ErrorCode = "PLUGIN_005" // 插件 RPC 超时

	// Dispatch 相关错误 (DISPATCH_xxx)
	ErrCodeDispatchNotStarted   ErrorCode = "DISPATCH_001" // Dispatcher 未启动
	ErrCodeDispatchNodeNotFound ErrorCode = "DISPATCH_002" // 目标节点未找到
	ErrCodeDispatchTimeout      ErrorCode = "DISPATCH_003" // 事件分发超时
	ErrCodeDispatchPayloadSize  ErrorCode = "DISPATCH_004" // Payload 超过大小限制
	ErrCodeDispatchRateLimited  ErrorCode = "DISPATCH_005" // 触发限流

	// DAG 相关错误 (DAG_xxx)
	ErrCodeDAGNotFound      ErrorCode = "DAG_001" // DAG 不存在
	ErrCodeDAGCycleDetected ErrorCode = "DAG_002" // DAG 检测到环
	ErrCodeDAGDuplicateNode ErrorCode = "DAG_003" // DAG 节点ID重复
	ErrCodeDAGNoSinkNode    ErrorCode = "DAG_004" // DAG 无 Sink 节点
	ErrCodeDAGBuildFailed   ErrorCode = "DAG_005" // DAG 构建失败

	// Runtime 相关错误 (RUNTIME_xxx)
	ErrCodeRuntimeInstanceNotFound ErrorCode = "RUNTIME_001" // 运行实例不存在
	ErrCodeRuntimeStartFailed      ErrorCode = "RUNTIME_002" // 运行实例启动失败
	ErrCodeRuntimeStopFailed       ErrorCode = "RUNTIME_003" // 运行实例停止失败

	// Config 相关错误 (CONFIG_xxx)
	ErrCodeConfigInvalid    ErrorCode = "CONFIG_001" // 配置无效
	ErrCodeConfigMissing    ErrorCode = "CONFIG_002" // 配置缺失
	ErrCodeConfigParseError ErrorCode = "CONFIG_003" // 配置解析错误

	// Auth 相关错误 (AUTH_xxx)
	ErrCodeAuthUnauthorized ErrorCode = "AUTH_001" // 未授权
	ErrCodeAuthForbidden    ErrorCode = "AUTH_002" // 禁止访问
	ErrCodeAuthTokenInvalid ErrorCode = "AUTH_003" // Token 无效
	ErrCodeAuthTokenExpired ErrorCode = "AUTH_004" // Token 过期

	// Internal 相关错误 (INTERNAL_xxx)
	ErrCodeInternalError ErrorCode = "INTERNAL_001" // 内部错误
	ErrCodeDatabaseError ErrorCode = "INTERNAL_002" // 数据库错误
	ErrCodeNATSError     ErrorCode = "INTERNAL_003" // NATS 错误
)

// AppError 应用错误
type AppError struct {
	Code       ErrorCode   `json:"code"`              // 错误码
	Message    string      `json:"message"`           // 错误消息
	Details    interface{} `json:"details,omitempty"` // 详细信息
	HTTPStatus int         `json:"-"`                 // HTTP 状态码
	Cause      error       `json:"-"`                 // 原始错误
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithDetails 添加详细信息
func (e *AppError) WithDetails(details interface{}) *AppError {
	e.Details = details
	return e
}

// WithCause 添加原始错误
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

// 预定义错误构造函数

// NewRuleNotFoundError 创建规则不存在错误
func NewRuleNotFoundError(ruleID string) *AppError {
	return &AppError{
		Code:       ErrCodeRuleNotFound,
		Message:    fmt.Sprintf("rule '%s' not found", ruleID),
		HTTPStatus: http.StatusNotFound,
	}
}

// NewRuleAlreadyExistsError 创建规则已存在错误
func NewRuleAlreadyExistsError(ruleID string) *AppError {
	return &AppError{
		Code:       ErrCodeRuleAlreadyExists,
		Message:    fmt.Sprintf("rule '%s' already exists", ruleID),
		HTTPStatus: http.StatusConflict,
	}
}

// NewRuleValidationError 创建规则验证错误
func NewRuleValidationError(details interface{}) *AppError {
	return &AppError{
		Code:       ErrCodeRuleValidation,
		Message:    "rule validation failed",
		Details:    details,
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewPluginNotFoundError 创建插件未找到错误
func NewPluginNotFoundError(pluginID string) *AppError {
	return &AppError{
		Code:       ErrCodePluginNotFound,
		Message:    fmt.Sprintf("plugin '%s' not found", pluginID),
		HTTPStatus: http.StatusNotFound,
	}
}

// NewPluginNotLoadedError 创建插件未加载错误
func NewPluginNotLoadedError(pluginID string) *AppError {
	return &AppError{
		Code:       ErrCodePluginNotLoaded,
		Message:    fmt.Sprintf("plugin '%s' is not loaded", pluginID),
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewDispatchNotStartedError 创建 Dispatcher 未启动错误
func NewDispatchNotStartedError(ruleID string) *AppError {
	return &AppError{
		Code:       ErrCodeDispatchNotStarted,
		Message:    fmt.Sprintf("dispatcher for rule '%s' is not started", ruleID),
		HTTPStatus: http.StatusServiceUnavailable,
	}
}

// NewDAGCycleDetectedError 创建 DAG 环检测错误
func NewDAGCycleDetectedError(cycleNodes []string) *AppError {
	return &AppError{
		Code:       ErrCodeDAGCycleDetected,
		Message:    "cycle detected in DAG",
		Details:    map[string]interface{}{"cycle_nodes": cycleNodes},
		HTTPStatus: http.StatusBadRequest,
	}
}

// NewInternalError 创建内部错误
func NewInternalError(cause error) *AppError {
	return &AppError{
		Code:       ErrCodeInternalError,
		Message:    "internal server error",
		Cause:      cause,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// NewDatabaseError 创建数据库错误
func NewDatabaseError(cause error) *AppError {
	return &AppError{
		Code:       ErrCodeDatabaseError,
		Message:    "database operation failed",
		Cause:      cause,
		HTTPStatus: http.StatusInternalServerError,
	}
}

// NewUnauthorizedError 创建未授权错误
func NewUnauthorizedError() *AppError {
	return &AppError{
		Code:       ErrCodeAuthUnauthorized,
		Message:    "authentication required",
		HTTPStatus: http.StatusUnauthorized,
	}
}

// NewForbiddenError 创建禁止访问错误
func NewForbiddenError() *AppError {
	return &AppError{
		Code:       ErrCodeAuthForbidden,
		Message:    "access forbidden",
		HTTPStatus: http.StatusForbidden,
	}
}

// IsAppError 检查是否为 AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// AsAppError 将 error 转换为 AppError
func AsAppError(err error) *AppError {
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	return NewInternalError(err)
}

// GetHTTPStatus 获取 HTTP 状态码
func GetHTTPStatus(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}
