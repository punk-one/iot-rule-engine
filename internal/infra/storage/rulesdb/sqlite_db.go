package rulesdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// SQLiteDB SQLite 数据库实现
type SQLiteDB struct {
	db                       *sql.DB
	path                     string
	watchers                 map[string]*sqliteWatcherState
	connectorWatchers        map[string]*sqliteWatcherState
	ruleSubscribers          map[int]sqliteRuleSubscriber
	nextRuleSubID            int
	ruleWatchCancel          context.CancelFunc
	ruleWatchDone            chan struct{}
	ruleWatchLoopStarts      int
	connectorSubscribers     map[int]sqliteConnectorSubscriber
	nextConnectorSubID       int
	connectorWatchCancel     context.CancelFunc
	connectorWatchDone       chan struct{}
	connectorWatchLoopStarts int
	mu                       sync.RWMutex
}

type sqliteWatcherState struct {
	lastModified time.Time
	lastData     []byte
}

type sqliteRuleSubscriber struct {
	onUpdate func(ruleID string, data []byte) error
	onDelete func(ruleID string) error
}

type sqliteConnectorSubscriber struct {
	onUpdate func(resourceID string, data []byte) error
	onDelete func(resourceID string) error
}

// OpenSQLite 打开 SQLite 数据库
func OpenSQLite(cfg Config) (*SQLiteDB, error) {
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

	// 检查路径是否是目录，如果是则报错（SQLite 需要文件路径）
	if info, err := os.Stat(dbPath); err == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("database path is a directory, not a file: %s (please remove or rename the directory to allow SQLite to create the database file)", dbPath)
		}
	}

	// 打开 SQLite 数据库（使用 WAL 模式提高并发性能）
	dsn := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000&_foreign_keys=on", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(1) // SQLite 单写多读
	db.SetMaxIdleConns(1)

	// 创建表
	if err := initSQLiteTables(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	pkglog.Info("SQLite database initialized at %s", dbPath)

	return &SQLiteDB{
		db:                   db,
		path:                 dbPath,
		watchers:             make(map[string]*sqliteWatcherState),
		connectorWatchers:    make(map[string]*sqliteWatcherState),
		ruleSubscribers:      make(map[int]sqliteRuleSubscriber),
		connectorSubscribers: make(map[int]sqliteConnectorSubscriber),
	}, nil
}

// initSQLiteTables 初始化表结构
func initSQLiteTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			flow TEXT NOT NULL,
			status TEXT DEFAULT 'draft',
			version INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			name TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			resource TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			rule_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			flow TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(rule_id, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_rule_id ON snapshots(rule_id)`,
		`CREATE TABLE IF NOT EXISTS gray_release (
			rule_id TEXT PRIMARY KEY,
			config TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// Close 关闭数据库
func (s *SQLiteDB) Close() error {
	s.stopRuleWatchLoop()
	s.stopConnectorWatchLoop()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Path 获取数据库路径
func (s *SQLiteDB) Path() string {
	return s.path
}

// SaveRule 保存规则
func (s *SQLiteDB) SaveRule(ruleFlow *schema.RuleFlow) error {
	if ruleFlow == nil || ruleFlow.ID == "" {
		return fmt.Errorf("rule id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin rule transaction: %w", err)
	}
	defer tx.Rollback()

	var oldFlow string
	err = tx.QueryRow("SELECT flow FROM rules WHERE id = ?", ruleFlow.ID).Scan(&oldFlow)
	exists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to load current rule: %w", err)
	}

	now := time.Now()
	if !exists {
		ruleFlow.CreatedAt = now
		ruleFlow.Version = 1
	} else {
		var current schema.RuleFlow
		if err := json.Unmarshal([]byte(oldFlow), &current); err != nil {
			return fmt.Errorf("failed to decode current rule: %w", err)
		}
		if ruleFlow.Version != current.Version {
			return fmt.Errorf("%w: expected version %d, got %d", ErrRuleVersionConflict, current.Version, ruleFlow.Version)
		}
		ruleFlow.CreatedAt = current.CreatedAt
		ruleFlow.Version = current.Version + 1
	}
	ruleFlow.UpdatedAt = now

	// 序列化 flow
	flowData, err := json.Marshal(ruleFlow)
	if err != nil {
		return fmt.Errorf("failed to marshal rule: %w", err)
	}

	if !exists {
		_, err = tx.Exec(`
		INSERT INTO rules (id, name, description, flow, status, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, ruleFlow.ID, ruleFlow.Name, ruleFlow.Description,
			string(flowData), ruleFlow.LifecycleStatus, ruleFlow.Version, ruleFlow.CreatedAt, ruleFlow.UpdatedAt)
	} else {
		result, updateErr := tx.Exec(`UPDATE rules SET name = ?, description = ?, flow = ?, status = ?, version = ?, updated_at = ? WHERE id = ? AND version = ?`,
			ruleFlow.Name, ruleFlow.Description, string(flowData), ruleFlow.LifecycleStatus, ruleFlow.Version,
			ruleFlow.UpdatedAt, ruleFlow.ID, ruleFlow.Version-1)
		err = updateErr
		if err == nil {
			rows, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				err = rowsErr
			} else if rows != 1 {
				err = ErrRuleVersionConflict
			}
		}
	}
	if err != nil {
		return fmt.Errorf("failed to save rule: %w", err)
	}

	if exists {
		snapshot := &schema.RuleSnapshot{
			ID:        fmt.Sprintf("%s-v%d-%d", ruleFlow.ID, ruleFlow.Version, now.Unix()),
			RuleID:    ruleFlow.ID,
			Version:   ruleFlow.Version,
			RuleFlow:  *ruleFlow,
			CreatedAt: now,
		}
		snapshotData, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("failed to marshal snapshot: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO snapshots (id, rule_id, version, flow, created_at) VALUES (?, ?, ?, ?, ?)`,
			snapshot.ID, snapshot.RuleID, snapshot.Version, string(snapshotData), snapshot.CreatedAt); err != nil {
			return fmt.Errorf("failed to save snapshot: %w", err)
		}
	}
	return tx.Commit()
}

// GetRule 获取规则
func (s *SQLiteDB) GetRule(ruleID string) (*schema.RuleFlow, error) {
	var flowData string
	err := s.db.QueryRow("SELECT flow FROM rules WHERE id = ?", ruleID).Scan(&flowData)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("rule %s not found", ruleID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	var ruleFlow schema.RuleFlow
	if err := json.Unmarshal([]byte(flowData), &ruleFlow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rule: %w", err)
	}

	return &ruleFlow, nil
}

// ListRules 列出所有规则
func (s *SQLiteDB) ListRules() ([]*schema.RuleFlow, error) {
	rows, err := s.db.Query("SELECT flow FROM rules")
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}
	defer rows.Close()

	var rules []*schema.RuleFlow
	for rows.Next() {
		var flowData string
		if err := rows.Scan(&flowData); err != nil {
			pkglog.Error("Failed to scan rule: %v", err)
			continue
		}

		var ruleFlow schema.RuleFlow
		if err := json.Unmarshal([]byte(flowData), &ruleFlow); err != nil {
			pkglog.Error("Failed to unmarshal rule: %v", err)
			continue
		}
		rules = append(rules, &ruleFlow)
	}

	return rules, rows.Err()
}

// DeleteRule 删除规则
func (s *SQLiteDB) DeleteRule(ruleID string) error {
	_, err := s.db.Exec("DELETE FROM rules WHERE id = ?", ruleID)
	return err
}

// RuleExists 检查规则是否存在
func (s *SQLiteDB) RuleExists(ruleID string) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM rules WHERE id = ?", ruleID).Scan(&count)
	return count > 0, err
}

// SaveConfig 保存系统配置
func (s *SQLiteDB) SaveConfig(configData []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO config (key, value, updated_at)
		VALUES ('system', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, string(configData))
	return err
}

// GetConfig 获取系统配置
func (s *SQLiteDB) GetConfig() ([]byte, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM config WHERE key = 'system'").Scan(&value)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("config not found")
	}
	if err != nil {
		return nil, err
	}
	return []byte(value), nil
}

// ConfigExists 检查配置是否存在
func (s *SQLiteDB) ConfigExists() (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM config WHERE key = 'system'").Scan(&count)
	return count > 0, err
}

// SaveConnectorResource 保存连接器资源定义
func (s *SQLiteDB) SaveConnectorResource(resource *sdk.ConnectorResource) error {
	if resource == nil {
		return fmt.Errorf("connector resource is nil")
	}
	if resource.ID == "" {
		return fmt.Errorf("connector resource id is required")
	}
	if resource.PluginID == "" {
		return fmt.Errorf("connector resource plugin_id is required")
	}

	data, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal connector resource: %w", err)
	}

	enabled := 0
	if resource.Enabled {
		enabled = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO connectors (id, plugin_id, name, enabled, resource, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			plugin_id = excluded.plugin_id,
			name = excluded.name,
			enabled = excluded.enabled,
			resource = excluded.resource,
			updated_at = CURRENT_TIMESTAMP
	`, resource.ID, resource.PluginID, resource.Name, enabled, string(data))
	if err != nil {
		return fmt.Errorf("failed to save connector resource: %w", err)
	}

	return nil
}

// GetConnectorResource 获取连接器资源定义
func (s *SQLiteDB) GetConnectorResource(resourceID string) (*sdk.ConnectorResource, error) {
	var raw string
	err := s.db.QueryRow("SELECT resource FROM connectors WHERE id = ?", resourceID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("connector resource %s not found", resourceID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connector resource: %w", err)
	}

	var resource sdk.ConnectorResource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connector resource: %w", err)
	}
	return &resource, nil
}

// ListConnectorResources 列出所有连接器资源定义
func (s *SQLiteDB) ListConnectorResources() ([]*sdk.ConnectorResource, error) {
	rows, err := s.db.Query("SELECT resource FROM connectors ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("failed to list connector resources: %w", err)
	}
	defer rows.Close()

	var resources []*sdk.ConnectorResource
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("failed to scan connector resource: %w", err)
		}
		var resource sdk.ConnectorResource
		if err := json.Unmarshal([]byte(raw), &resource); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connector resource: %w", err)
		}
		resources = append(resources, &resource)
	}

	return resources, rows.Err()
}

// DeleteConnectorResource 删除连接器资源定义
func (s *SQLiteDB) DeleteConnectorResource(resourceID string) error {
	_, err := s.db.Exec("DELETE FROM connectors WHERE id = ?", resourceID)
	if err != nil {
		return fmt.Errorf("failed to delete connector resource: %w", err)
	}
	return nil
}

// WatchRules 监听规则变更（轮询机制）
func (s *SQLiteDB) WatchRules(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) (context.CancelFunc, error) {
	s.mu.Lock()
	if s.ruleWatchCancel == nil {
		rules, err := s.ListRules()
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize watcher: %w", err)
		}
		s.watchers = make(map[string]*sqliteWatcherState, len(rules))
		for _, rule := range rules {
			data, _ := json.Marshal(rule)
			s.watchers[rule.ID] = &sqliteWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), data...),
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		s.ruleWatchCancel = cancel
		s.ruleWatchDone = done
		s.ruleWatchLoopStarts++
		go s.runRuleWatchLoop(ctx, done)
		pkglog.Info("Started watching SQLite rules (polling interval: 1s)")
	}
	subID := s.nextRuleSubID
	s.nextRuleSubID++
	s.ruleSubscribers[subID] = sqliteRuleSubscriber{onUpdate: onUpdate, onDelete: onDelete}
	s.mu.Unlock()

	return func() {
		s.removeRuleSubscriber(subID)
	}, nil
}

// WatchConnectorResources 监听连接器资源变更（轮询机制）
func (s *SQLiteDB) WatchConnectorResources(onUpdate func(resourceID string, data []byte) error, onDelete func(resourceID string) error) (context.CancelFunc, error) {
	s.mu.Lock()
	if s.connectorWatchCancel == nil {
		resources, err := s.ListConnectorResources()
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize connector watcher: %w", err)
		}
		s.connectorWatchers = make(map[string]*sqliteWatcherState, len(resources))
		for _, resource := range resources {
			data, _ := json.Marshal(resource)
			s.connectorWatchers[resource.ID] = &sqliteWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), data...),
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		s.connectorWatchCancel = cancel
		s.connectorWatchDone = done
		s.connectorWatchLoopStarts++
		go s.runConnectorWatchLoop(ctx, done)
		pkglog.Info("Started watching SQLite connector resources (polling interval: 1s)")
	}
	subID := s.nextConnectorSubID
	s.nextConnectorSubID++
	s.connectorSubscribers[subID] = sqliteConnectorSubscriber{onUpdate: onUpdate, onDelete: onDelete}
	s.mu.Unlock()

	return func() {
		s.removeConnectorSubscriber(subID)
	}, nil
}

// pollSQLiteChanges 轮询检查变更
func (s *SQLiteDB) pollSQLiteChanges() {
	// 获取当前规则
	currentRules := make(map[string][]byte)
	rows, err := s.db.Query("SELECT id, flow FROM rules")
	if err != nil {
		pkglog.Error("Failed to poll rules: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, flow string
		if err := rows.Scan(&id, &flow); err != nil {
			continue
		}
		currentRules[id] = []byte(flow)
	}

	s.mu.Lock()
	subscribers := make([]sqliteRuleSubscriber, 0, len(s.ruleSubscribers))
	for _, sub := range s.ruleSubscribers {
		subscribers = append(subscribers, sub)
	}
	type ruleChange struct {
		ruleID string
		data   []byte
	}
	var updates []ruleChange
	var deletes []string

	// 检查更新
	for ruleID, currentData := range currentRules {
		state, exists := s.watchers[ruleID]
		if !exists {
			updates = append(updates, ruleChange{ruleID: ruleID, data: append([]byte(nil), currentData...)})
			continue
		}

		if !bytesEqual(state.lastData, currentData) {
			updates = append(updates, ruleChange{ruleID: ruleID, data: append([]byte(nil), currentData...)})
		}
	}

	// 检查删除
	for ruleID := range s.watchers {
		if _, exists := currentRules[ruleID]; !exists {
			deletes = append(deletes, ruleID)
		}
	}
	s.mu.Unlock()

	for _, update := range updates {
		delivered := true
		for _, sub := range subscribers {
			if sub.onUpdate == nil {
				continue
			}
			if err := sub.onUpdate(update.ruleID, update.data); err != nil {
				delivered = false
				pkglog.Error("Error handling rule update for %s: %v", update.ruleID, err)
			}
		}
		if delivered {
			s.mu.Lock()
			s.watchers[update.ruleID] = &sqliteWatcherState{lastModified: time.Now(), lastData: append([]byte(nil), update.data...)}
			s.mu.Unlock()
		}
	}
	for _, ruleID := range deletes {
		delivered := true
		for _, sub := range subscribers {
			if sub.onDelete == nil {
				continue
			}
			if err := sub.onDelete(ruleID); err != nil {
				delivered = false
				pkglog.Error("Error handling rule delete for %s: %v", ruleID, err)
			}
		}
		if delivered {
			s.mu.Lock()
			delete(s.watchers, ruleID)
			s.mu.Unlock()
		}
	}
}

func (s *SQLiteDB) pollSQLiteConnectorChanges() {
	currentResources := make(map[string][]byte)
	rows, err := s.db.Query("SELECT id, resource FROM connectors")
	if err != nil {
		pkglog.Error("Failed to poll connector resources: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		currentResources[id] = []byte(raw)
	}

	s.mu.Lock()
	subscribers := make([]sqliteConnectorSubscriber, 0, len(s.connectorSubscribers))
	for _, sub := range s.connectorSubscribers {
		subscribers = append(subscribers, sub)
	}
	type resourceChange struct {
		resourceID string
		data       []byte
	}
	var updates []resourceChange
	var deletes []string

	for resourceID, currentData := range currentResources {
		state, exists := s.connectorWatchers[resourceID]
		if !exists {
			updates = append(updates, resourceChange{resourceID: resourceID, data: append([]byte(nil), currentData...)})
			continue
		}

		if !bytesEqual(state.lastData, currentData) {
			updates = append(updates, resourceChange{resourceID: resourceID, data: append([]byte(nil), currentData...)})
		}
	}

	for resourceID := range s.connectorWatchers {
		if _, exists := currentResources[resourceID]; !exists {
			deletes = append(deletes, resourceID)
		}
	}
	s.mu.Unlock()

	for _, update := range updates {
		delivered := true
		for _, sub := range subscribers {
			if sub.onUpdate == nil {
				continue
			}
			if err := sub.onUpdate(update.resourceID, update.data); err != nil {
				delivered = false
				pkglog.Error("Error handling connector resource update for %s: %v", update.resourceID, err)
			}
		}
		if delivered {
			s.mu.Lock()
			s.connectorWatchers[update.resourceID] = &sqliteWatcherState{lastModified: time.Now(), lastData: append([]byte(nil), update.data...)}
			s.mu.Unlock()
		}
	}
	for _, resourceID := range deletes {
		delivered := true
		for _, sub := range subscribers {
			if sub.onDelete == nil {
				continue
			}
			if err := sub.onDelete(resourceID); err != nil {
				delivered = false
				pkglog.Error("Error handling connector resource delete for %s: %v", resourceID, err)
			}
		}
		if delivered {
			s.mu.Lock()
			delete(s.connectorWatchers, resourceID)
			s.mu.Unlock()
		}
	}
}

func (s *SQLiteDB) runRuleWatchLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pkglog.Debug("SQLite rule watcher stopped")
			return
		case <-ticker.C:
			s.pollSQLiteChanges()
		}
	}
}

func (s *SQLiteDB) runConnectorWatchLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pkglog.Debug("SQLite connector watcher stopped")
			return
		case <-ticker.C:
			s.pollSQLiteConnectorChanges()
		}
	}
}

func (s *SQLiteDB) removeRuleSubscriber(id int) {
	s.mu.Lock()
	delete(s.ruleSubscribers, id)
	shouldStop := len(s.ruleSubscribers) == 0 && s.ruleWatchCancel != nil
	cancel := s.ruleWatchCancel
	done := s.ruleWatchDone
	if shouldStop {
		s.ruleWatchCancel = nil
		s.ruleWatchDone = nil
	}
	s.mu.Unlock()

	if shouldStop {
		cancel()
		<-done
	}
}

func (s *SQLiteDB) removeConnectorSubscriber(id int) {
	s.mu.Lock()
	delete(s.connectorSubscribers, id)
	shouldStop := len(s.connectorSubscribers) == 0 && s.connectorWatchCancel != nil
	cancel := s.connectorWatchCancel
	done := s.connectorWatchDone
	if shouldStop {
		s.connectorWatchCancel = nil
		s.connectorWatchDone = nil
	}
	s.mu.Unlock()

	if shouldStop {
		cancel()
		<-done
	}
}

func (s *SQLiteDB) stopRuleWatchLoop() {
	s.mu.Lock()
	cancel := s.ruleWatchCancel
	done := s.ruleWatchDone
	s.ruleWatchCancel = nil
	s.ruleWatchDone = nil
	s.ruleSubscribers = make(map[int]sqliteRuleSubscriber)
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *SQLiteDB) stopConnectorWatchLoop() {
	s.mu.Lock()
	cancel := s.connectorWatchCancel
	done := s.connectorWatchDone
	s.connectorWatchCancel = nil
	s.connectorWatchDone = nil
	s.connectorSubscribers = make(map[int]sqliteConnectorSubscriber)
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
}

// SaveSnapshot 保存快照
func (s *SQLiteDB) SaveSnapshot(snapshot *schema.RuleSnapshot) error {
	flowData, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO snapshots (id, rule_id, version, flow, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(rule_id, version) DO UPDATE SET flow = excluded.flow
	`, snapshot.ID, snapshot.RuleID, snapshot.Version, string(flowData), snapshot.CreatedAt)
	return err
}

// GetSnapshot 获取快照
func (s *SQLiteDB) GetSnapshot(ruleID string, version int) (*schema.RuleSnapshot, error) {
	var flowData string
	err := s.db.QueryRow("SELECT flow FROM snapshots WHERE rule_id = ? AND version = ?", ruleID, version).Scan(&flowData)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("snapshot %s:%d not found", ruleID, version)
	}
	if err != nil {
		return nil, err
	}

	var snapshot schema.RuleSnapshot
	if err := json.Unmarshal([]byte(flowData), &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// ListSnapshots 列出快照
func (s *SQLiteDB) ListSnapshots(ruleID string) ([]*schema.RuleSnapshot, error) {
	rows, err := s.db.Query("SELECT flow FROM snapshots WHERE rule_id = ? ORDER BY version DESC", ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []*schema.RuleSnapshot
	for rows.Next() {
		var flowData string
		if err := rows.Scan(&flowData); err != nil {
			continue
		}
		var snapshot schema.RuleSnapshot
		if err := json.Unmarshal([]byte(flowData), &snapshot); err != nil {
			continue
		}
		snapshots = append(snapshots, &snapshot)
	}
	return snapshots, rows.Err()
}

// GetLatestSnapshot 获取最新快照
func (s *SQLiteDB) GetLatestSnapshot(ruleID string) (*schema.RuleSnapshot, error) {
	snapshots, err := s.ListSnapshots(ruleID)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshot found for rule %s", ruleID)
	}
	return snapshots[0], nil
}

// SaveGrayReleaseConfig 保存灰度发布配置
func (s *SQLiteDB) SaveGrayReleaseConfig(config *schema.GrayReleaseConfig) error {
	config.UpdatedAt = time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal gray release config: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO gray_release (rule_id, config, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(rule_id) DO UPDATE SET config = excluded.config, updated_at = excluded.updated_at
	`, config.RuleID, string(data), config.CreatedAt, config.UpdatedAt)
	return err
}

// GetGrayReleaseConfig 获取灰度发布配置
func (s *SQLiteDB) GetGrayReleaseConfig(ruleID string) (*schema.GrayReleaseConfig, error) {
	var configData string
	err := s.db.QueryRow("SELECT config FROM gray_release WHERE rule_id = ?", ruleID).Scan(&configData)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("gray release config for rule %s not found", ruleID)
	}
	if err != nil {
		return nil, err
	}

	var config schema.GrayReleaseConfig
	if err := json.Unmarshal([]byte(configData), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// DeleteGrayReleaseConfig 删除灰度发布配置
func (s *SQLiteDB) DeleteGrayReleaseConfig(ruleID string) error {
	_, err := s.db.Exec("DELETE FROM gray_release WHERE rule_id = ?", ruleID)
	return err
}

// 确保 SQLiteDB 实现了 RulesDB 接口
var _ RulesDB = (*SQLiteDB)(nil)
