package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Backend State Backend 接口
// 支持三种实现：Memory、Redis、Local DB
type Backend interface {
	// Get 获取状态
	Get(ctx context.Context, key string) ([]byte, error)

	// Set 设置状态
	Set(ctx context.Context, key string, value []byte) error

	// SetWithTTL 设置带过期时间的状态
	SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete 删除状态
	Delete(ctx context.Context, key string) error

	// Close 关闭 Backend
	Close() error
}

// StateManager State 管理器
// 负责根据配置创建和提供 State Backend
type StateManager struct {
	backend Backend
}

// NewStateManager 创建 State Manager
func NewStateManager(backend Backend) *StateManager {
	return &StateManager{
		backend: backend,
	}
}

// GetBackend 获取 Backend
func (m *StateManager) GetBackend() Backend {
	return m.backend
}

// Close 关闭 State Manager
func (m *StateManager) Close() error {
	if m.backend != nil {
		return m.backend.Close()
	}
	return nil
}

// GetState 获取状态（反序列化）
func (m *StateManager) GetState(ctx context.Context, key string, state interface{}) error {
	data, err := m.backend.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("state not found: %s", key)
	}
	return json.Unmarshal(data, state)
}

// SetState 设置状态（序列化）
func (m *StateManager) SetState(ctx context.Context, key string, state interface{}) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return m.backend.Set(ctx, key, data)
}

// SetStateWithTTL 设置带过期时间的状态（序列化）
func (m *StateManager) SetStateWithTTL(ctx context.Context, key string, state interface{}, ttl time.Duration) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return m.backend.SetWithTTL(ctx, key, data, ttl)
}

// DeleteState 删除状态
func (m *StateManager) DeleteState(ctx context.Context, key string) error {
	return m.backend.Delete(ctx, key)
}

// GenerateStateKey 生成 State Key（规范 3.2）
// state_key = hash(rule_id + node_id + concat(keys[]))
func GenerateStateKey(ruleID, nodeID string, keys []string) string {
	// 简化实现：使用字符串拼接（实际应该使用 hash）
	keyStr := fmt.Sprintf("%s:%s:%s", ruleID, nodeID, fmt.Sprintf("%v", keys))
	return fmt.Sprintf("state:%s", keyStr)
}
