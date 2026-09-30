package rulesdb

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// DatabaseConfigFields PostgreSQL 数据库配置字段（避免循环导入）
type DatabaseConfigFields struct {
	Host                   string
	Port                   int
	Name                   string
	User                   string
	Password               string
	SSLMode                string
	MaxIdleConns           int
	MaxOpenConns           int
	ConnMaxLifetimeMinutes int
}

// PostgresDB PostgreSQL 数据库实现
type PostgresDB struct {
	db                       *gorm.DB
	dsn                      string
	watchers                 map[string]*postgresWatcherState
	connectorWatchers        map[string]*postgresWatcherState
	ruleSubscribers          map[int]postgresRuleSubscriber
	nextRuleSubID            int
	ruleWatchCancel          context.CancelFunc
	ruleWatchDone            chan struct{}
	ruleWatchLoopStarts      int
	connectorSubscribers     map[int]postgresConnectorSubscriber
	nextConnectorSubID       int
	connectorWatchCancel     context.CancelFunc
	connectorWatchDone       chan struct{}
	connectorWatchLoopStarts int
	mu                       sync.RWMutex
}

type postgresWatcherState struct {
	lastModified time.Time
	lastData     []byte
}

type postgresRuleSubscriber struct {
	onUpdate func(ruleID string, data []byte) error
	onDelete func(ruleID string) error
}

type postgresConnectorSubscriber struct {
	onUpdate func(resourceID string, data []byte) error
	onDelete func(resourceID string) error
}

// ReRule 规则表模型
type ReRule struct {
	ID          string    `gorm:"primaryKey;column:id" json:"id"`
	Name        string    `gorm:"column:name;not null" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	Flow        string    `gorm:"column:flow;type:text;not null" json:"flow"` // JSON 序列化的 RuleFlow
	Status      string    `gorm:"column:status;default:'draft'" json:"status"`
	Version     int       `gorm:"column:version;default:1" json:"version"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamptz;default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:timestamptz;default:current_timestamp" json:"updated_at"`
}

func (ReRule) TableName() string {
	return "re_rules"
}

// ReConfig 配置表模型
type ReConfig struct {
	Key       string    `gorm:"primaryKey;column:key" json:"key"`
	Value     string    `gorm:"column:value;type:text;not null" json:"value"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;default:current_timestamp" json:"updated_at"`
}

func (ReConfig) TableName() string {
	return "re_config"
}

// ReConnector 连接器资源表模型
type ReConnector struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	PluginID  string    `gorm:"column:plugin_id;not null;index:idx_re_connectors_plugin_id" json:"plugin_id"`
	Name      string    `gorm:"column:name" json:"name"`
	Enabled   bool      `gorm:"column:enabled;not null;default:true" json:"enabled"`
	Resource  string    `gorm:"column:resource;type:text;not null" json:"resource"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;default:current_timestamp" json:"updated_at"`
}

func (ReConnector) TableName() string {
	return "re_connectors"
}

// ReSnapshot 快照表模型
type ReSnapshot struct {
	ID        string    `gorm:"primaryKey;column:id" json:"id"`
	RuleID    string    `gorm:"column:rule_id;not null;index:idx_re_snapshots_rule_id" json:"rule_id"`
	Version   int       `gorm:"column:version;not null;uniqueIndex:idx_snapshots_rule_version" json:"version"`
	Flow      string    `gorm:"column:flow;type:text;not null" json:"flow"` // JSON 序列化的 RuleSnapshot
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;default:current_timestamp" json:"created_at"`
}

func (ReSnapshot) TableName() string {
	return "re_snapshots"
}

// ReGrayRelease 灰度发布配置表模型
type ReGrayRelease struct {
	RuleID    string    `gorm:"primaryKey;column:rule_id" json:"rule_id"`
	Config    string    `gorm:"column:config;type:text;not null" json:"config"` // JSON 序列化的 GrayReleaseConfig
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz;default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz;default:current_timestamp" json:"updated_at"`
}

func (ReGrayRelease) TableName() string {
	return "re_gray_release"
}

// OpenPostgres 打开 PostgreSQL 数据库
// dbConfig 应该是 *config.DatabaseConfig 类型，使用 interface{} 避免循环导入
func OpenPostgres(cfg Config, dbConfig interface{}) (*PostgresDB, error) {
	if dbConfig == nil {
		return nil, fmt.Errorf("database config is required for postgres storage type")
	}

	// 使用反射提取字段，避免循环导入
	dbConfigFields, err := extractDatabaseConfigFields(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to extract database config fields: %w", err)
	}

	// 构建 DSN（键值对格式）
	dsn := buildPostgresDSN(dbConfigFields)

	// 打开数据库连接
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	// 获取底层的 sql.DB 以设置连接池参数
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// 设置连接池参数
	if dbConfigFields.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(dbConfigFields.MaxIdleConns)
	} else {
		sqlDB.SetMaxIdleConns(10) // 默认值
	}

	if dbConfigFields.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(dbConfigFields.MaxOpenConns)
	} else {
		sqlDB.SetMaxOpenConns(100) // 默认值
	}

	if dbConfigFields.ConnMaxLifetimeMinutes > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(dbConfigFields.ConnMaxLifetimeMinutes) * time.Minute)
	} else {
		sqlDB.SetConnMaxLifetime(60 * time.Minute) // 默认值
	}

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 初始化表结构
	if err := initPostgresTables(db); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	pkglog.Info("PostgreSQL database initialized at %s:%d/%s", dbConfigFields.Host, dbConfigFields.Port, dbConfigFields.Name)

	return &PostgresDB{
		db:                   db,
		dsn:                  dsn,
		watchers:             make(map[string]*postgresWatcherState),
		connectorWatchers:    make(map[string]*postgresWatcherState),
		ruleSubscribers:      make(map[int]postgresRuleSubscriber),
		connectorSubscribers: make(map[int]postgresConnectorSubscriber),
	}, nil
}

// extractDatabaseConfigFields 使用反射提取数据库配置字段（避免循环导入）
func extractDatabaseConfigFields(dbConfig interface{}) (*DatabaseConfigFields, error) {
	v := reflect.ValueOf(dbConfig)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("database config must be a struct or pointer to struct")
	}

	fields := &DatabaseConfigFields{}
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		switch field.Name {
		case "Host":
			if value.Kind() == reflect.String {
				fields.Host = value.String()
			}
		case "Port":
			if value.Kind() == reflect.Int {
				fields.Port = int(value.Int())
			}
		case "Name":
			if value.Kind() == reflect.String {
				fields.Name = value.String()
			}
		case "User":
			if value.Kind() == reflect.String {
				fields.User = value.String()
			}
		case "Password":
			if value.Kind() == reflect.String {
				fields.Password = value.String()
			}
		case "SSLMode":
			if value.Kind() == reflect.String {
				fields.SSLMode = value.String()
			}
		case "MaxIdleConns":
			if value.Kind() == reflect.Int {
				fields.MaxIdleConns = int(value.Int())
			}
		case "MaxOpenConns":
			if value.Kind() == reflect.Int {
				fields.MaxOpenConns = int(value.Int())
			}
		case "ConnMaxLifetimeMinutes":
			if value.Kind() == reflect.Int {
				fields.ConnMaxLifetimeMinutes = int(value.Int())
			}
		}
	}

	return fields, nil
}

// buildPostgresDSN 构建 PostgreSQL DSN（键值对格式）
func buildPostgresDSN(dbConfig *DatabaseConfigFields) string {
	// GORM PostgreSQL 驱动使用键值对格式
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s",
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Name,
	)

	// 添加 SSL 模式
	if dbConfig.SSLMode != "" {
		dsn += fmt.Sprintf(" sslmode=%s", dbConfig.SSLMode)
	} else {
		dsn += " sslmode=disable"
	}

	return dsn
}

// initPostgresTables 初始化表结构
func initPostgresTables(db *gorm.DB) error {
	// 使用 GORM AutoMigrate 自动创建表
	if err := db.AutoMigrate(&ReRule{}, &ReConfig{}, &ReConnector{}, &ReSnapshot{}, &ReGrayRelease{}); err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}

	// 确保索引存在（GORM AutoMigrate 应该已经创建，但为了保险起见）
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_re_snapshots_rule_id ON re_snapshots(rule_id)").Error; err != nil {
		// 索引可能已存在，忽略错误
		pkglog.Debug("Index creation result: %v", err)
	}

	return nil
}

// Close 关闭数据库
func (p *PostgresDB) Close() error {
	p.stopRuleWatchLoop()
	p.stopConnectorWatchLoop()
	if p.db != nil {
		sqlDB, err := p.db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// Path 获取数据库路径（返回 DSN）
func (p *PostgresDB) Path() string {
	return p.dsn
}

// SaveRule 保存规则
func (p *PostgresDB) SaveRule(ruleFlow *schema.RuleFlow) error {
	// 检查规则是否存在
	exists, err := p.RuleExists(ruleFlow.ID)
	if err != nil {
		return err
	}

	var oldVersion int
	if exists {
		oldRule, err := p.GetRule(ruleFlow.ID)
		if err == nil {
			oldVersion = oldRule.Version
		}
	}

	// 设置时间戳和版本
	now := time.Now()
	if !exists {
		if ruleFlow.CreatedAt.IsZero() {
			ruleFlow.CreatedAt = now
		}
		if ruleFlow.Version == 0 {
			ruleFlow.Version = 1
		}
	} else {
		if ruleFlow.Version == oldVersion {
			ruleFlow.Version = oldVersion + 1
		}
	}
	ruleFlow.UpdatedAt = now

	// 序列化 flow
	flowData, err := json.Marshal(ruleFlow)
	if err != nil {
		return fmt.Errorf("failed to marshal rule: %w", err)
	}

	// 保存规则（使用 GORM 的 Upsert）
	rule := ReRule{
		ID:          ruleFlow.ID,
		Name:        ruleFlow.Name,
		Description: ruleFlow.Description,
		Flow:        string(flowData),
		Status:      ruleFlow.LifecycleStatus,
		Version:     ruleFlow.Version,
		CreatedAt:   ruleFlow.CreatedAt,
		UpdatedAt:   ruleFlow.UpdatedAt,
	}

	if err := p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "flow", "status", "version", "updated_at"}),
	}).Create(&rule).Error; err != nil {
		return fmt.Errorf("failed to save rule: %w", err)
	}

	// 创建快照
	if exists && ruleFlow.Version > oldVersion {
		snapshot := &schema.RuleSnapshot{
			ID:        fmt.Sprintf("%s-v%d-%d", ruleFlow.ID, ruleFlow.Version, now.Unix()),
			RuleID:    ruleFlow.ID,
			Version:   ruleFlow.Version,
			RuleFlow:  *ruleFlow,
			CreatedAt: now,
		}
		if err := p.SaveSnapshot(snapshot); err != nil {
			pkglog.Error("Failed to create snapshot for rule %s version %d: %v", ruleFlow.ID, ruleFlow.Version, err)
		}
	}

	return nil
}

// GetRule 获取规则
func (p *PostgresDB) GetRule(ruleID string) (*schema.RuleFlow, error) {
	var rule ReRule
	if err := p.db.Where("id = ?", ruleID).First(&rule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("rule %s not found", ruleID)
		}
		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	var ruleFlow schema.RuleFlow
	if err := json.Unmarshal([]byte(rule.Flow), &ruleFlow); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rule: %w", err)
	}

	return &ruleFlow, nil
}

// ListRules 列出所有规则
func (p *PostgresDB) ListRules() ([]*schema.RuleFlow, error) {
	var rules []ReRule
	if err := p.db.Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	var result []*schema.RuleFlow
	for _, rule := range rules {
		var ruleFlow schema.RuleFlow
		if err := json.Unmarshal([]byte(rule.Flow), &ruleFlow); err != nil {
			pkglog.Error("Failed to unmarshal rule %s: %v", rule.ID, err)
			continue
		}
		result = append(result, &ruleFlow)
	}

	return result, nil
}

// DeleteRule 删除规则
func (p *PostgresDB) DeleteRule(ruleID string) error {
	if err := p.db.Where("id = ?", ruleID).Delete(&ReRule{}).Error; err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	return nil
}

// RuleExists 检查规则是否存在
func (p *PostgresDB) RuleExists(ruleID string) (bool, error) {
	var count int64
	if err := p.db.Model(&ReRule{}).Where("id = ?", ruleID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// SaveConfig 保存系统配置
func (p *PostgresDB) SaveConfig(configData []byte) error {
	config := ReConfig{
		Key:       "system",
		Value:     string(configData),
		UpdatedAt: time.Now(),
	}

	if err := p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&config).Error; err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// GetConfig 获取系统配置
func (p *PostgresDB) GetConfig() ([]byte, error) {
	var config ReConfig
	result := p.db.Where("key = ?", "system").Limit(1).Find(&config)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("config not found")
	}
	if config.Key == "" {
		// Find on a struct does not return ErrRecordNotFound, so guard the empty row explicitly.
		return nil, fmt.Errorf("config not found")
	}

	return []byte(config.Value), nil
}

// ConfigExists 检查配置是否存在
func (p *PostgresDB) ConfigExists() (bool, error) {
	var count int64
	if err := p.db.Model(&ReConfig{}).Where("key = ?", "system").Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// SaveConnectorResource 保存连接器资源定义
func (p *PostgresDB) SaveConnectorResource(resource *sdk.ConnectorResource) error {
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

	record := ReConnector{
		ID:        resource.ID,
		PluginID:  resource.PluginID,
		Name:      resource.Name,
		Enabled:   resource.Enabled,
		Resource:  string(data),
		UpdatedAt: time.Now(),
	}

	if err := p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"plugin_id", "name", "enabled", "resource", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return fmt.Errorf("failed to save connector resource: %w", err)
	}

	return nil
}

// GetConnectorResource 获取连接器资源定义
func (p *PostgresDB) GetConnectorResource(resourceID string) (*sdk.ConnectorResource, error) {
	var record ReConnector
	if err := p.db.Where("id = ?", resourceID).First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("connector resource %s not found", resourceID)
		}
		return nil, fmt.Errorf("failed to get connector resource: %w", err)
	}

	var resource sdk.ConnectorResource
	if err := json.Unmarshal([]byte(record.Resource), &resource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connector resource: %w", err)
	}
	return &resource, nil
}

// ListConnectorResources 列出所有连接器资源定义
func (p *PostgresDB) ListConnectorResources() ([]*sdk.ConnectorResource, error) {
	var records []ReConnector
	if err := p.db.Order("id ASC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list connector resources: %w", err)
	}

	resources := make([]*sdk.ConnectorResource, 0, len(records))
	for _, record := range records {
		var resource sdk.ConnectorResource
		if err := json.Unmarshal([]byte(record.Resource), &resource); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connector resource %s: %w", record.ID, err)
		}
		resources = append(resources, &resource)
	}
	return resources, nil
}

// DeleteConnectorResource 删除连接器资源定义
func (p *PostgresDB) DeleteConnectorResource(resourceID string) error {
	if err := p.db.Where("id = ?", resourceID).Delete(&ReConnector{}).Error; err != nil {
		return fmt.Errorf("failed to delete connector resource: %w", err)
	}
	return nil
}

// WatchRules 监听规则变更（轮询机制）
func (p *PostgresDB) WatchRules(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) (context.CancelFunc, error) {
	p.mu.Lock()
	if p.ruleWatchCancel == nil {
		rules, err := p.ListRules()
		if err != nil {
			p.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize watcher: %w", err)
		}
		p.watchers = make(map[string]*postgresWatcherState, len(rules))
		for _, rule := range rules {
			data, _ := json.Marshal(rule)
			p.watchers[rule.ID] = &postgresWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), data...),
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		p.ruleWatchCancel = cancel
		p.ruleWatchDone = done
		p.ruleWatchLoopStarts++
		go p.runRuleWatchLoop(ctx, done)
		pkglog.Info("Started watching PostgreSQL rules (polling interval: 1s)")
	}
	subID := p.nextRuleSubID
	p.nextRuleSubID++
	p.ruleSubscribers[subID] = postgresRuleSubscriber{onUpdate: onUpdate, onDelete: onDelete}
	p.mu.Unlock()

	return func() {
		p.removeRuleSubscriber(subID)
	}, nil
}

// WatchConnectorResources 监听连接器资源变更（轮询机制）
func (p *PostgresDB) WatchConnectorResources(onUpdate func(resourceID string, data []byte) error, onDelete func(resourceID string) error) (context.CancelFunc, error) {
	p.mu.Lock()
	if p.connectorWatchCancel == nil {
		resources, err := p.ListConnectorResources()
		if err != nil {
			p.mu.Unlock()
			return nil, fmt.Errorf("failed to initialize connector watcher: %w", err)
		}
		p.connectorWatchers = make(map[string]*postgresWatcherState, len(resources))
		for _, resource := range resources {
			data, _ := json.Marshal(resource)
			p.connectorWatchers[resource.ID] = &postgresWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), data...),
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		p.connectorWatchCancel = cancel
		p.connectorWatchDone = done
		p.connectorWatchLoopStarts++
		go p.runConnectorWatchLoop(ctx, done)
		pkglog.Info("Started watching PostgreSQL connector resources (polling interval: 1s)")
	}
	subID := p.nextConnectorSubID
	p.nextConnectorSubID++
	p.connectorSubscribers[subID] = postgresConnectorSubscriber{onUpdate: onUpdate, onDelete: onDelete}
	p.mu.Unlock()

	return func() {
		p.removeConnectorSubscriber(subID)
	}, nil
}

// pollPostgresChanges 轮询检查变更
func (p *PostgresDB) pollPostgresChanges() {
	// 获取当前规则
	currentRules := make(map[string][]byte)
	var rules []ReRule
	if err := p.db.Find(&rules).Error; err != nil {
		pkglog.Error("Failed to poll rules: %v", err)
		return
	}

	for _, rule := range rules {
		currentRules[rule.ID] = []byte(rule.Flow)
	}

	p.mu.Lock()
	subscribers := make([]postgresRuleSubscriber, 0, len(p.ruleSubscribers))
	for _, sub := range p.ruleSubscribers {
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
		state, exists := p.watchers[ruleID]
		if !exists {
			p.watchers[ruleID] = &postgresWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), currentData...),
			}
			updates = append(updates, ruleChange{ruleID: ruleID, data: append([]byte(nil), currentData...)})
			continue
		}

		if !bytesEqual(state.lastData, currentData) {
			state.lastModified = time.Now()
			state.lastData = make([]byte, len(currentData))
			copy(state.lastData, currentData)
			updates = append(updates, ruleChange{ruleID: ruleID, data: append([]byte(nil), currentData...)})
		}
	}

	// 检查删除
	for ruleID := range p.watchers {
		if _, exists := currentRules[ruleID]; !exists {
			delete(p.watchers, ruleID)
			deletes = append(deletes, ruleID)
		}
	}
	p.mu.Unlock()

	for _, update := range updates {
		for _, sub := range subscribers {
			if sub.onUpdate == nil {
				continue
			}
			if err := sub.onUpdate(update.ruleID, update.data); err != nil {
				pkglog.Error("Error handling rule update for %s: %v", update.ruleID, err)
			}
		}
	}
	for _, ruleID := range deletes {
		for _, sub := range subscribers {
			if sub.onDelete == nil {
				continue
			}
			if err := sub.onDelete(ruleID); err != nil {
				pkglog.Error("Error handling rule delete for %s: %v", ruleID, err)
			}
		}
	}
}

func (p *PostgresDB) pollPostgresConnectorChanges() {
	currentResources := make(map[string][]byte)
	var records []ReConnector
	if err := p.db.Find(&records).Error; err != nil {
		pkglog.Error("Failed to poll connector resources: %v", err)
		return
	}

	for _, record := range records {
		currentResources[record.ID] = []byte(record.Resource)
	}

	p.mu.Lock()
	subscribers := make([]postgresConnectorSubscriber, 0, len(p.connectorSubscribers))
	for _, sub := range p.connectorSubscribers {
		subscribers = append(subscribers, sub)
	}
	type resourceChange struct {
		resourceID string
		data       []byte
	}
	var updates []resourceChange
	var deletes []string

	for resourceID, currentData := range currentResources {
		state, exists := p.connectorWatchers[resourceID]
		if !exists {
			p.connectorWatchers[resourceID] = &postgresWatcherState{
				lastModified: time.Now(),
				lastData:     append([]byte(nil), currentData...),
			}
			updates = append(updates, resourceChange{resourceID: resourceID, data: append([]byte(nil), currentData...)})
			continue
		}

		if !bytesEqual(state.lastData, currentData) {
			state.lastModified = time.Now()
			state.lastData = make([]byte, len(currentData))
			copy(state.lastData, currentData)
			updates = append(updates, resourceChange{resourceID: resourceID, data: append([]byte(nil), currentData...)})
		}
	}

	for resourceID := range p.connectorWatchers {
		if _, exists := currentResources[resourceID]; !exists {
			delete(p.connectorWatchers, resourceID)
			deletes = append(deletes, resourceID)
		}
	}
	p.mu.Unlock()

	for _, update := range updates {
		for _, sub := range subscribers {
			if sub.onUpdate == nil {
				continue
			}
			if err := sub.onUpdate(update.resourceID, update.data); err != nil {
				pkglog.Error("Error handling connector resource update for %s: %v", update.resourceID, err)
			}
		}
	}
	for _, resourceID := range deletes {
		for _, sub := range subscribers {
			if sub.onDelete == nil {
				continue
			}
			if err := sub.onDelete(resourceID); err != nil {
				pkglog.Error("Error handling connector resource delete for %s: %v", resourceID, err)
			}
		}
	}
}

func (p *PostgresDB) runRuleWatchLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pkglog.Debug("PostgreSQL rule watcher stopped")
			return
		case <-ticker.C:
			p.pollPostgresChanges()
		}
	}
}

func (p *PostgresDB) runConnectorWatchLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			pkglog.Debug("PostgreSQL connector watcher stopped")
			return
		case <-ticker.C:
			p.pollPostgresConnectorChanges()
		}
	}
}

func (p *PostgresDB) removeRuleSubscriber(id int) {
	p.mu.Lock()
	delete(p.ruleSubscribers, id)
	shouldStop := len(p.ruleSubscribers) == 0 && p.ruleWatchCancel != nil
	cancel := p.ruleWatchCancel
	done := p.ruleWatchDone
	if shouldStop {
		p.ruleWatchCancel = nil
		p.ruleWatchDone = nil
	}
	p.mu.Unlock()

	if shouldStop {
		cancel()
		<-done
	}
}

func (p *PostgresDB) removeConnectorSubscriber(id int) {
	p.mu.Lock()
	delete(p.connectorSubscribers, id)
	shouldStop := len(p.connectorSubscribers) == 0 && p.connectorWatchCancel != nil
	cancel := p.connectorWatchCancel
	done := p.connectorWatchDone
	if shouldStop {
		p.connectorWatchCancel = nil
		p.connectorWatchDone = nil
	}
	p.mu.Unlock()

	if shouldStop {
		cancel()
		<-done
	}
}

func (p *PostgresDB) stopRuleWatchLoop() {
	p.mu.Lock()
	cancel := p.ruleWatchCancel
	done := p.ruleWatchDone
	p.ruleWatchCancel = nil
	p.ruleWatchDone = nil
	p.ruleSubscribers = make(map[int]postgresRuleSubscriber)
	p.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
}

func (p *PostgresDB) stopConnectorWatchLoop() {
	p.mu.Lock()
	cancel := p.connectorWatchCancel
	done := p.connectorWatchDone
	p.connectorWatchCancel = nil
	p.connectorWatchDone = nil
	p.connectorSubscribers = make(map[int]postgresConnectorSubscriber)
	p.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
}

// SaveSnapshot 保存快照
func (p *PostgresDB) SaveSnapshot(snapshot *schema.RuleSnapshot) error {
	flowData, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	snap := ReSnapshot{
		ID:        snapshot.ID,
		RuleID:    snapshot.RuleID,
		Version:   snapshot.Version,
		Flow:      string(flowData),
		CreatedAt: snapshot.CreatedAt,
	}

	if err := p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rule_id"}, {Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"flow"}),
	}).Create(&snap).Error; err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// GetSnapshot 获取快照
func (p *PostgresDB) GetSnapshot(ruleID string, version int) (*schema.RuleSnapshot, error) {
	var snap ReSnapshot
	if err := p.db.Where("rule_id = ? AND version = ?", ruleID, version).First(&snap).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("snapshot %s:%d not found", ruleID, version)
		}
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	var snapshot schema.RuleSnapshot
	if err := json.Unmarshal([]byte(snap.Flow), &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// ListSnapshots 列出快照
func (p *PostgresDB) ListSnapshots(ruleID string) ([]*schema.RuleSnapshot, error) {
	var snaps []ReSnapshot
	if err := p.db.Where("rule_id = ?", ruleID).Order("version DESC").Find(&snaps).Error; err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	var snapshots []*schema.RuleSnapshot
	for _, snap := range snaps {
		var snapshot schema.RuleSnapshot
		if err := json.Unmarshal([]byte(snap.Flow), &snapshot); err != nil {
			pkglog.Error("Failed to unmarshal snapshot %s: %v", snap.ID, err)
			continue
		}
		snapshots = append(snapshots, &snapshot)
	}

	return snapshots, nil
}

// GetLatestSnapshot 获取最新快照
func (p *PostgresDB) GetLatestSnapshot(ruleID string) (*schema.RuleSnapshot, error) {
	snapshots, err := p.ListSnapshots(ruleID)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshot found for rule %s", ruleID)
	}
	return snapshots[0], nil
}

// SaveGrayReleaseConfig 保存灰度发布配置
func (p *PostgresDB) SaveGrayReleaseConfig(config *schema.GrayReleaseConfig) error {
	config.UpdatedAt = time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now()
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal gray release config: %w", err)
	}

	grayRelease := ReGrayRelease{
		RuleID:    config.RuleID,
		Config:    string(data),
		CreatedAt: config.CreatedAt,
		UpdatedAt: config.UpdatedAt,
	}

	if err := p.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rule_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"config", "updated_at"}),
	}).Create(&grayRelease).Error; err != nil {
		return fmt.Errorf("failed to save gray release config: %w", err)
	}

	return nil
}

// GetGrayReleaseConfig 获取灰度发布配置
func (p *PostgresDB) GetGrayReleaseConfig(ruleID string) (*schema.GrayReleaseConfig, error) {
	var grayRelease ReGrayRelease
	if err := p.db.Where("rule_id = ?", ruleID).First(&grayRelease).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("gray release config for rule %s not found", ruleID)
		}
		return nil, fmt.Errorf("failed to get gray release config: %w", err)
	}

	var config schema.GrayReleaseConfig
	if err := json.Unmarshal([]byte(grayRelease.Config), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gray release config: %w", err)
	}

	return &config, nil
}

// DeleteGrayReleaseConfig 删除灰度发布配置
func (p *PostgresDB) DeleteGrayReleaseConfig(ruleID string) error {
	if err := p.db.Where("rule_id = ?", ruleID).Delete(&ReGrayRelease{}).Error; err != nil {
		return fmt.Errorf("failed to delete gray release config: %w", err)
	}
	return nil
}

// 确保 PostgresDB 实现了 RulesDB 接口
var _ RulesDB = (*PostgresDB)(nil)
