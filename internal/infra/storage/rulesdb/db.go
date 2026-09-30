package rulesdb

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"go.etcd.io/bbolt"
)

// Config 数据库配置
type Config struct {
	DataDir        string        // 数据目录
	Timeout        time.Duration // 导出超时时间
	StorageType    string        // sqlite | postgres
	DatabaseConfig interface{}   // PostgreSQL 数据库配置（*config.DatabaseConfig），仅当 StorageType=postgres 时使用
}

// DB 封装 bbolt 数据库操作
// 移除单例模式和 refCount，由 Engine 作为唯一 Owner
type DB struct {
	db       *bbolt.DB
	path     string
	watchers map[string]*watcherState // ruleID -> watcher state
	mu       sync.RWMutex
}

// watcherState 存储 watcher 的状态信息
type watcherState struct {
	lastModified time.Time
	lastData     []byte
}

// Open 打开数据库（根据配置的 StorageType 选择实现）
func Open(cfg Config) (RulesDB, error) {
	return OpenByType(cfg, cfg.StorageType)
}

// openBolt 底层打开 BoltDB（内部使用）
func openBolt(cfg Config) (*DB, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}

	// 构建数据库路径
	dbPath := cfg.DataDir
	if dbPath == "" {
		dbPath = "data/rules.db"
	} else {
		dbPath = filepath.Join(cfg.DataDir, "rules.db")
	}

	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// 打开数据库
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{
		Timeout: cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 创建 bucket
	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(BucketRules); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(BucketConfig); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(BucketSnapshots); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(BucketGrayRelease); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	pkglog.Info("Database initialized at %s", dbPath)

	return &DB{
		db:       db,
		path:     dbPath,
		watchers: make(map[string]*watcherState),
	}, nil
}

// Close 关闭数据库
func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

// Bolt 获取底层 BoltDB 实例（用于高级操作）
func (d *DB) Bolt() *bbolt.DB {
	return d.db
}

// Path 获取数据库路径
func (d *DB) Path() string {
	return d.path
}

// SaveRule 保存规则
// 如果版本号增加，自动创建快照
func (d *DB) SaveRule(ruleFlow *schema.RuleFlow) error {
	// 检查规则是否存在
	exists, err := d.RuleExists(ruleFlow.ID)
	if err != nil {
		return err
	}

	var oldVersion int
	if exists {
		oldRule, err := d.GetRule(ruleFlow.ID)
		if err == nil {
			oldVersion = oldRule.Version
		}
	}

	// 如果规则不存在，设置 CreatedAt
	if !exists {
		if ruleFlow.CreatedAt.IsZero() {
			ruleFlow.CreatedAt = time.Now()
		}
		// 新规则默认版本为 1
		if ruleFlow.Version == 0 {
			ruleFlow.Version = 1
		}
	} else {
		// 如果版本号未变化，自动递增
		if ruleFlow.Version == oldVersion {
			ruleFlow.Version = oldVersion + 1
		}
	}
	// 总是更新 UpdatedAt
	ruleFlow.UpdatedAt = time.Now()

	data, err := json.Marshal(ruleFlow)
	if err != nil {
		return fmt.Errorf("failed to marshal rule: %w", err)
	}

	// 保存规则
	err = d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		return bucket.Put([]byte(ruleFlow.ID), data)
	})
	if err != nil {
		return err
	}

	// 如果版本号增加，自动创建快照
	if exists && ruleFlow.Version > oldVersion {
		snapshot := &schema.RuleSnapshot{
			ID:        fmt.Sprintf("%s-v%d-%d", ruleFlow.ID, ruleFlow.Version, time.Now().Unix()),
			RuleID:    ruleFlow.ID,
			Version:   ruleFlow.Version,
			RuleFlow:  *ruleFlow,
			CreatedAt: time.Now(),
		}
		if err := d.SaveSnapshot(snapshot); err != nil {
			pkglog.Error("Failed to create snapshot for rule %s version %d: %v", ruleFlow.ID, ruleFlow.Version, err)
			// 不返回错误，快照失败不影响规则保存
		} else {
			pkglog.Debug("Created snapshot for rule %s version %d", ruleFlow.ID, ruleFlow.Version)
		}
	}

	return nil
}

// GetRule 获取规则
func (d *DB) GetRule(ruleID string) (*schema.RuleFlow, error) {
	var ruleFlow schema.RuleFlow
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		data := bucket.Get([]byte(ruleID))
		if data == nil {
			return fmt.Errorf("rule %s not found", ruleID)
		}

		return json.Unmarshal(data, &ruleFlow)
	})

	if err != nil {
		return nil, err
	}

	return &ruleFlow, nil
}

// ListRules 列出所有规则
func (d *DB) ListRules() ([]*schema.RuleFlow, error) {
	var rules []*schema.RuleFlow
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		return bucket.ForEach(func(k, v []byte) error {
			var ruleFlow schema.RuleFlow
			if err := json.Unmarshal(v, &ruleFlow); err != nil {
				pkglog.Error("Failed to unmarshal rule %s: %v", string(k), err)
				return nil // 跳过损坏的规则
			}
			rules = append(rules, &ruleFlow)
			return nil
		})
	})

	return rules, err
}

// DeleteRule 删除规则
func (d *DB) DeleteRule(ruleID string) error {
	return d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		return bucket.Delete([]byte(ruleID))
	})
}

// RuleExists 检查规则是否存在
func (d *DB) RuleExists(ruleID string) (bool, error) {
	exists := false
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		data := bucket.Get([]byte(ruleID))
		exists = data != nil
		return nil
	})

	return exists, err
}

// SaveConfig 保存系统配置
func (d *DB) SaveConfig(configData []byte) error {
	return d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketConfig)
		return bucket.Put([]byte("system"), configData)
	})
}

// GetConfig 获取系统配置
func (d *DB) GetConfig() ([]byte, error) {
	var configData []byte
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketConfig)
		data := bucket.Get([]byte("system"))
		if data == nil {
			return fmt.Errorf("config not found")
		}
		configData = make([]byte, len(data))
		copy(configData, data)
		return nil
	})
	return configData, err
}

// ConfigExists 检查配置是否存在
func (d *DB) ConfigExists() (bool, error) {
	exists := false
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketConfig)
		data := bucket.Get([]byte("system"))
		exists = data != nil
		return nil
	})
	return exists, err
}

// WatchRules 监听所有规则的变更（基于轮询机制）
// 由于 BoltDB 不支持文件系统级别的 watch，我们使用轮询机制
// onUpdate: 当规则更新时调用，参数为 ruleID 和规则数据
// onDelete: 当规则删除时调用，参数为 ruleID
// 返回一个 context.CancelFunc，用于停止监听
func (d *DB) WatchRules(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化 watcher 状态：记录所有现有规则的当前状态
	initialRules := make(map[string]*watcherState)
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		return bucket.ForEach(func(k, v []byte) error {
			ruleID := string(k)
			initialRules[ruleID] = &watcherState{
				lastModified: time.Now(), // 使用当前时间作为初始时间
				lastData:     make([]byte, len(v)),
			}
			copy(initialRules[ruleID].lastData, v)
			return nil
		})
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize watcher state: %w", err)
	}

	d.mu.Lock()
	for ruleID, state := range initialRules {
		d.watchers[ruleID] = state
	}
	d.mu.Unlock()

	// 启动轮询 goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Second) // 每秒轮询一次
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				pkglog.Debug("Rule watcher stopped")
				return
			case <-ticker.C:
				d.pollChanges(onUpdate, onDelete)
			}
		}
	}()

	pkglog.Info("Started watching rules in database (polling interval: 1s)")
	return cancel, nil
}

// pollChanges 轮询检查规则变更
func (d *DB) pollChanges(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 获取当前数据库中的所有规则
	currentRules := make(map[string][]byte)
	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketRules)
		return bucket.ForEach(func(k, v []byte) error {
			ruleID := string(k)
			data := make([]byte, len(v))
			copy(data, v)
			currentRules[ruleID] = data
			return nil
		})
	})
	if err != nil {
		pkglog.Error("Failed to poll rules: %v", err)
		return
	}

	// 检查更新和新增的规则
	for ruleID, currentData := range currentRules {
		state, exists := d.watchers[ruleID]
		if !exists {
			// 新规则
			d.watchers[ruleID] = &watcherState{
				lastModified: time.Now(),
				lastData:     make([]byte, len(currentData)),
			}
			copy(d.watchers[ruleID].lastData, currentData)
			// 新规则必须触发 onUpdate，否则“创建+部署”在同一轮询窗口内会被漏掉。
			if err := onUpdate(ruleID, currentData); err != nil {
				pkglog.Error("Error handling new rule update for %s: %v", ruleID, err)
			} else {
				pkglog.Debug("New rule %s detected and loaded", ruleID)
			}
			continue
		}

		// 检查数据是否变化
		if !bytesEqual(state.lastData, currentData) {
			// 规则已更新
			state.lastModified = time.Now()
			state.lastData = make([]byte, len(currentData))
			copy(state.lastData, currentData)

			if err := onUpdate(ruleID, currentData); err != nil {
				pkglog.Error("Error handling rule update for %s: %v", ruleID, err)
			} else {
				pkglog.Debug("Rule %s updated", ruleID)
			}
		}
	}

	// 检查删除的规则
	for ruleID := range d.watchers {
		if _, exists := currentRules[ruleID]; !exists {
			// 规则已删除
			delete(d.watchers, ruleID)

			if err := onDelete(ruleID); err != nil {
				pkglog.Error("Error handling rule delete for %s: %v", ruleID, err)
			} else {
				pkglog.Debug("Rule %s deleted", ruleID)
			}
		}
	}
}

// bytesEqual 比较两个字节切片是否相等
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SaveSnapshot 保存规则快照
func (d *DB) SaveSnapshot(snapshot *schema.RuleSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// 使用 ruleID:version 作为 key，确保同一规则的同一版本只有一个快照
	key := fmt.Sprintf("%s:%d", snapshot.RuleID, snapshot.Version)

	return d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketSnapshots)
		return bucket.Put([]byte(key), data)
	})
}

// GetSnapshot 获取指定规则的指定版本快照
func (d *DB) GetSnapshot(ruleID string, version int) (*schema.RuleSnapshot, error) {
	var snapshot schema.RuleSnapshot
	key := fmt.Sprintf("%s:%d", ruleID, version)

	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketSnapshots)
		data := bucket.Get([]byte(key))
		if data == nil {
			return fmt.Errorf("snapshot %s:%d not found", ruleID, version)
		}
		return json.Unmarshal(data, &snapshot)
	})

	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// ListSnapshots 列出指定规则的所有快照（按版本降序）
func (d *DB) ListSnapshots(ruleID string) ([]*schema.RuleSnapshot, error) {
	var snapshots []*schema.RuleSnapshot
	prefix := []byte(ruleID + ":")

	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketSnapshots)
		cursor := bucket.Cursor()

		for k, v := cursor.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, v = cursor.Next() {
			var snapshot schema.RuleSnapshot
			if err := json.Unmarshal(v, &snapshot); err != nil {
				pkglog.Error("Failed to unmarshal snapshot %s: %v", string(k), err)
				continue
			}
			snapshots = append(snapshots, &snapshot)
		}

		// 按版本降序排序
		for i := 0; i < len(snapshots); i++ {
			for j := i + 1; j < len(snapshots); j++ {
				if snapshots[i].Version < snapshots[j].Version {
					snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
				}
			}
		}

		return nil
	})

	return snapshots, err
}

// GetLatestSnapshot 获取指定规则的最新快照
func (d *DB) GetLatestSnapshot(ruleID string) (*schema.RuleSnapshot, error) {
	snapshots, err := d.ListSnapshots(ruleID)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshot found for rule %s", ruleID)
	}
	return snapshots[0], nil // ListSnapshots 已按版本降序排序
}

// SaveGrayReleaseConfig 保存灰度发布配置
func (d *DB) SaveGrayReleaseConfig(config *schema.GrayReleaseConfig) error {
	config.UpdatedAt = time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal gray release config: %w", err)
	}

	return d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketGrayRelease)
		return bucket.Put([]byte(config.RuleID), data)
	})
}

// GetGrayReleaseConfig 获取灰度发布配置
func (d *DB) GetGrayReleaseConfig(ruleID string) (*schema.GrayReleaseConfig, error) {
	var config schema.GrayReleaseConfig

	err := d.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketGrayRelease)
		data := bucket.Get([]byte(ruleID))
		if data == nil {
			return fmt.Errorf("gray release config for rule %s not found", ruleID)
		}
		return json.Unmarshal(data, &config)
	})

	if err != nil {
		return nil, err
	}
	return &config, nil
}

// DeleteGrayReleaseConfig 删除灰度发布配置
func (d *DB) DeleteGrayReleaseConfig(ruleID string) error {
	return d.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(BucketGrayRelease)
		return bucket.Delete([]byte(ruleID))
	})
}
