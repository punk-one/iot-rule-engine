package utils

import (
	"github.com/google/uuid"
)

// GenerateID 生成唯一ID
func GenerateID() string {
	return uuid.New().String()
}

// GenerateTraceID 生成追踪ID
func GenerateTraceID() string {
	return uuid.New().String()
}
