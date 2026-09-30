package state

import (
	"fmt"

	"github.com/punk-one/iot-rule-engine/internal/infra/config"
)

// NewBackendFromConfig 根据配置创建 State Backend
// state_backend 用于 Stateful 插件（如 ChangeDetect、WindowAggregate）的状态缓存
// 支持类型：memory | redis
// 注意：这与规则存储（manager.storage_type: sqlite | postgres）是两个不同的配置
// 当 type=redis 时，必须提供全局 Redis 配置
func NewBackendFromConfig(cfg config.StateBackendConfig, globalCfg *config.Config) (Backend, error) {
	backendType := cfg.Type
	if backendType == "" {
		backendType = "memory" // 默认使用 memory
	}

	switch backendType {
	case "memory":
		return NewMemoryBackend(), nil

	case "redis":
		// 使用全局 Redis 配置
		if globalCfg == nil {
			return nil, fmt.Errorf("global config is required for redis backend")
		}
		return NewRedisBackendFromGlobalConfig(globalCfg.Redis)

	default:
		return nil, fmt.Errorf("unsupported state backend type: %s (supported: memory, redis)", backendType)
	}
}
