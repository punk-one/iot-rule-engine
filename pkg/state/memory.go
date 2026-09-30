package state

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
}

// MemoryBackend 内存 Backend（默认）
type MemoryBackend struct {
	store map[string]memoryEntry
	mutex sync.RWMutex
}

// NewMemoryBackend 创建内存 Backend
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		store: make(map[string]memoryEntry),
	}
}

// Get 获取状态
func (m *MemoryBackend) Get(ctx context.Context, key string) ([]byte, error) {
	m.mutex.RLock()
	entry, exists := m.store[key]
	if exists && entry.isExpired(time.Now()) {
		m.mutex.RUnlock()
		m.mutex.Lock()
		delete(m.store, key)
		m.mutex.Unlock()
		return nil, nil
	}
	defer m.mutex.RUnlock()

	if !exists {
		return nil, nil
	}

	result := make([]byte, len(entry.value))
	copy(result, entry.value)
	return result, nil
}

// Set 设置状态
func (m *MemoryBackend) Set(ctx context.Context, key string, value []byte) error {
	return m.SetWithTTL(ctx, key, value, 0)
}

// SetWithTTL 设置带过期时间的状态
func (m *MemoryBackend) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	storeValue := make([]byte, len(value))
	copy(storeValue, value)

	entry := memoryEntry{value: storeValue}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	m.store[key] = entry
	return nil
}

// Delete 删除状态
func (m *MemoryBackend) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.store, key)
	return nil
}

func (e memoryEntry) isExpired(now time.Time) bool {
	return !e.expiresAt.IsZero() && !now.Before(e.expiresAt)
}

// Close 关闭 Backend（内存 Backend 无需清理）
func (m *MemoryBackend) Close() error {
	return nil
}
