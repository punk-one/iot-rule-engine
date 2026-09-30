package runtime_kv

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// KVClient 封装运行态 Key-Value 存储客户端
//
// 架构说明：
// - runtime_kv 用于运行态状态存储（数据面），与 rulesdb（控制面）分离
// - 支持 NATS KV 实现（未来可扩展支持内存实现）
// - 用于存储插件注册表、临时状态、offset/watermark 等运行态数据
// - 生命周期：进程内/可丢，与 rulesdb（跨进程/重启）不同
type KVClient struct {
	kv nats.KeyValue
}

var (
	// 全局 KV 客户端实例
	globalKV *KVClient
)

// InitKV 初始化运行态 KV 存储
// runtime_kv 仅用于运行时状态存储，不再用于规则配置存储
//
// 存储职责硬分层：
// - rules.db (BoltDB): 规则配置（唯一事实源）
// - runtime_kv (NATS KV): 运行时状态（非事实源，进程内/可丢）
//
// Bucket 语义：
// - RUNTIME_STATE: 运行时状态（替代旧的 CONFIG_RULES，保留向后兼容）
// - PLUGIN_REGISTRY: 插件注册表
func InitKV(js nats.JetStreamContext) error {
	// 创建 RUNTIME_STATE bucket（替代 CONFIG_RULES）
	// 先尝试创建新的 bucket
	runtimeKV, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "RUNTIME_STATE",
		Description: "Runtime state storage (replaces CONFIG_RULES, DO NOT use for rule config)",
		TTL:         0, // 永不过期
	})
	if err != nil {
		// 如果 bucket 已存在，尝试获取它
		if isBucketAlreadyExistsErr(err) {
			runtimeKV, err = js.KeyValue("RUNTIME_STATE")
			if err != nil {
				return fmt.Errorf("failed to get RUNTIME_STATE bucket: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create RUNTIME_STATE bucket: %w", err)
		}
	}

	// 创建或获取 PLUGIN_REGISTRY bucket
	_, err = js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "PLUGIN_REGISTRY",
		Description: "Plugin registry (replaces CONFIG_PLUGINS)",
		TTL:         0,
	})
	if err != nil {
		// 如果 bucket 已存在，忽略错误
		if !isBucketAlreadyExistsErr(err) {
			return fmt.Errorf("failed to create PLUGIN_REGISTRY bucket: %w", err)
		}
	}

	globalKV = &KVClient{
		kv: runtimeKV, // 默认使用 RUNTIME_STATE
	}

	pkglog.Info("Runtime KV storage initialized successfully (RUNTIME_STATE bucket)")
	return nil
}

func isBucketAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, nats.ErrStreamNameAlreadyInUse) ||
		strings.Contains(err.Error(), "stream name already in use")
}

// GetKV 获取全局运行态 KV 客户端
func GetKV() *KVClient {
	return globalKV
}

// Put 存储运行态状态（JSON 序列化）
func (c *KVClient) Put(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	_, err = c.kv.Put(key, data)
	if err != nil {
		return fmt.Errorf("failed to put key %s: %w", key, err)
	}

	pkglog.Debug("Put key %s to runtime KV store", key)
	return nil
}

// PutRaw 存储原始字节数据（不进行 JSON 序列化）
func (c *KVClient) PutRaw(key string, data []byte) error {
	_, err := c.kv.Put(key, data)
	if err != nil {
		return fmt.Errorf("failed to put key %s: %w", key, err)
	}

	pkglog.Debug("Put key %s to runtime KV store (raw)", key)
	return nil
}

// Get 获取运行态状态（JSON 反序列化）
func (c *KVClient) Get(key string, value interface{}) error {
	entry, err := c.kv.Get(key)
	if err != nil {
		return fmt.Errorf("failed to get key %s: %w", key, err)
	}

	if err := json.Unmarshal(entry.Value(), value); err != nil {
		return fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
	}

	return nil
}

// Delete 删除运行态状态
func (c *KVClient) Delete(key string) error {
	if err := c.kv.Delete(key); err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	pkglog.Debug("Deleted key %s from runtime KV store", key)
	return nil
}

// GetAll 获取所有运行态状态（用于 Pull on Start）
// 注意：NATS KV 的 WatchAll 会先发送所有现有键，然后监听新变更
func (c *KVClient) GetAll(onEntry func(key string, value []byte) error) error {
	watcher, err := c.kv.WatchAll()
	if err != nil {
		return fmt.Errorf("failed to watch all keys: %w", err)
	}
	defer watcher.Stop()

	// 读取所有现有配置（WatchAll 会先发送所有现有键）
	processed := make(map[string]bool)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case entry := <-watcher.Updates():
			if entry == nil {
				// 通道关闭，所有现有键已读取完毕
				return nil
			}

			key := entry.Key()
			// 只处理每个键一次（避免重复）
			if !processed[key] && entry.Operation() == nats.KeyValuePut {
				processed[key] = true
				if err := onEntry(key, entry.Value()); err != nil {
					return err
				}
			}
		case <-timeout.C:
			// 超时后停止读取（已读取的键足够）
			pkglog.Debug("GetAll timeout, stopping")
			return nil
		}
	}
}

// WatchConfig 监听单个 key 的运行态状态变更
func (c *KVClient) WatchConfig(key string, onUpdate func([]byte) error, onDelete func() error) error {
	watcher, err := c.kv.Watch(key)
	if err != nil {
		return fmt.Errorf("failed to watch key %s: %w", key, err)
	}

	go func() {
		for {
			select {
			case entry := <-watcher.Updates():
				if entry == nil {
					// 通道关闭
					return
				}

				if entry.Operation() == nats.KeyValuePut {
					pkglog.Debug("Key %s updated", key)
					if err := onUpdate(entry.Value()); err != nil {
						pkglog.Error("Error handling update for key %s: %v", key, err)
					}
				} else if entry.Operation() == nats.KeyValueDelete {
					pkglog.Debug("Key %s deleted", key)
					if err := onDelete(); err != nil {
						pkglog.Error("Error handling delete for key %s: %v", key, err)
					}
				}
			}
		}
	}()

	pkglog.Info("Watching key %s for changes", key)
	return nil
}

// WatchAll 监听整个 bucket 的运行态状态变更
// 注意：NATS KV 的 WatchAll() 在发送完所有现有键值对后，通道会关闭（返回 nil）
// 这是正常行为，表示所有现有数据已发送完毕，之后会继续监听新变更
// 因此，当通道关闭时，应该继续监听，而不是重启 watcher
func (c *KVClient) WatchAll(onUpdate func(string, []byte) error, onDelete func(string) error) error {
	go func() {
		retryDelay := 1 * time.Second
		maxRetryDelay := 30 * time.Second

		for {
			watcher, err := c.kv.WatchAll()
			if err != nil {
				pkglog.Error("Failed to watch all keys, retrying in %v: %v", retryDelay, err)
				time.Sleep(retryDelay)
				// 指数退避
				retryDelay *= 2
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
				}
				continue
			}

			// 重置重试延迟（成功创建 watcher）
			retryDelay = 1 * time.Second
			initialDataSent := false

			// 持续监听，直到 watcher 真正停止（错误或手动停止）
			for entry := range watcher.Updates() {
				if entry == nil {
					// 通道关闭（nil）表示所有现有数据已发送完毕
					// 这是正常行为，NATS KV 会继续监听新变更
					// 只有在初始数据发送完毕后才记录日志，避免频繁日志
					if !initialDataSent {
						pkglog.Debug("KV watcher: initial data sent, now watching for changes...")
						initialDataSent = true
					}
					// 继续监听，不要退出循环
					continue
				}

				key := entry.Key()
				if entry.Operation() == nats.KeyValuePut {
					pkglog.Debug("Key %s updated", key)
					if err := onUpdate(key, entry.Value()); err != nil {
						pkglog.Error("Error handling update for key %s: %v", key, err)
						// 继续处理其他键，不中断 watcher
					}
				} else if entry.Operation() == nats.KeyValueDelete {
					pkglog.Debug("Key %s deleted", key)
					if err := onDelete(key); err != nil {
						pkglog.Error("Error handling delete for key %s: %v", key, err)
						// 继续处理其他键，不中断 watcher
					}
				}
			}

			// 只有当通道真正关闭（watcher 停止）时才停止并重启
			watcher.Stop()
			pkglog.Warn("KV watcher stopped unexpectedly, restarting in %v...", retryDelay)
			time.Sleep(retryDelay)
		}
	}()

	pkglog.Info("Watching all keys in bucket")
	return nil
}

// SetBucket 切换到指定的 bucket
func (c *KVClient) SetBucket(js nats.JetStreamContext, bucketName string) error {
	kv, err := js.KeyValue(bucketName)
	if err != nil {
		return fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	c.kv = kv
	return nil
}
