// Package errors 提供统一的错误处理框架
package errors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse API 错误响应结构
type ErrorResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// HandleError 统一处理错误并返回 JSON 响应
func HandleError(c *gin.Context, err error) {
	appErr := AsAppError(err)

	c.JSON(appErr.HTTPStatus, ErrorResponse{
		Code:    string(appErr.Code),
		Message: appErr.Message,
		Details: appErr.Details,
	})
}

// HandleErrorWithStatus 使用指定状态码处理错误
func HandleErrorWithStatus(c *gin.Context, status int, err error) {
	appErr := AsAppError(err)
	appErr.HTTPStatus = status

	c.JSON(status, ErrorResponse{
		Code:    string(appErr.Code),
		Message: appErr.Message,
		Details: appErr.Details,
	})
}

// RespondBadRequest 返回 400 错误
func RespondBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Code:    string(ErrCodeRuleInvalidFormat),
		Message: message,
	})
}

// RespondNotFound 返回 404 错误
func RespondNotFound(c *gin.Context, resourceType, resourceID string) {
	c.JSON(http.StatusNotFound, ErrorResponse{
		Code:    string(ErrCodeRuleNotFound),
		Message: resourceType + " '" + resourceID + "' not found",
	})
}

// RespondConflict 返回 409 错误
func RespondConflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, ErrorResponse{
		Code:    string(ErrCodeRuleAlreadyExists),
		Message: message,
	})
}

// RespondUnauthorized 返回 401 错误
func RespondUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, ErrorResponse{
		Code:    string(ErrCodeAuthUnauthorized),
		Message: "authentication required",
	})
}

// RespondForbidden 返回 403 错误
func RespondForbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, ErrorResponse{
		Code:    string(ErrCodeAuthForbidden),
		Message: "access forbidden",
	})
}

// RespondInternalError 返回 500 错误
func RespondInternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Code:    string(ErrCodeInternalError),
		Message: "internal server error",
	})
}

// WrapDatabaseError 包装数据库错误
func WrapDatabaseError(err error) *AppError {
	return NewDatabaseError(err)
}

// WrapValidationError 包装验证错误
func WrapValidationError(details interface{}) *AppError {
	return NewRuleValidationError(details)
}
