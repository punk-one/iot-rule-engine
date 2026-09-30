package state

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
)

// RedisBackend Redis Backend 实现 (v1.6)
type RedisBackend struct {
	client *redis.Client
	prefix string
}

// RedisConfig Redis 配置参数
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// NewRedisBackendFromGlobalConfig 从全局配置创建 Redis Backend
func NewRedisBackendFromGlobalConfig(cfg config.RedisConfig) (*RedisBackend, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("redis host is required")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s:%d: %w", cfg.Host, cfg.Port, err)
	}

	return &RedisBackend{
		client: client,
		prefix: "punk:state:",
	}, nil
}

// Get 获取状态
func (r *RedisBackend) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := r.client.Get(ctx, r.prefix+key).Bytes()
	if err == redis.Nil {
		return nil, nil // key 不存在返回 nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}
	return result, nil
}

// Set 设置状态
func (r *RedisBackend) Set(ctx context.Context, key string, value []byte) error {
	return r.SetWithTTL(ctx, key, value, 0)
}

// SetWithTTL 设置带过期时间的状态
func (r *RedisBackend) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	err := r.client.Set(ctx, r.prefix+key, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("redis set with ttl error: %w", err)
	}
	return nil
}

// Delete 删除状态
func (r *RedisBackend) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, r.prefix+key).Err()
	if err != nil {
		return fmt.Errorf("redis delete error: %w", err)
	}
	return nil
}

// Close 关闭 Backend
func (r *RedisBackend) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
