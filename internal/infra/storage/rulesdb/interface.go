package rulesdb

import (
	"context"
	"errors"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// ErrRuleVersionConflict is returned when a stale editor attempts to overwrite
// a newer rule revision.
var ErrRuleVersionConflict = errors.New("rule version conflict")

// RulesDB 规则数据库接口
// 支持多种后端实现：SQLite、PostgreSQL
type RulesDB interface {
	// Close 关闭数据库
	Close() error

	// Path 获取数据库路径
	Path() string

	// ==================== 规则操作 ====================

	// SaveRule 保存规则
	SaveRule(ruleFlow *schema.RuleFlow) error

	// GetRule 获取规则
	GetRule(ruleID string) (*schema.RuleFlow, error)

	// ListRules 列出所有规则
	ListRules() ([]*schema.RuleFlow, error)

	// DeleteRule 删除规则
	DeleteRule(ruleID string) error

	// RuleExists 检查规则是否存在
	RuleExists(ruleID string) (bool, error)

	// ==================== 配置操作 ====================

	// SaveConfig 保存系统配置
	SaveConfig(configData []byte) error

	// GetConfig 获取系统配置
	GetConfig() ([]byte, error)

	// ConfigExists 检查配置是否存在
	ConfigExists() (bool, error)

	// ==================== Connector Resource 操作 ====================

	// SaveConnectorResource 保存连接器资源定义
	SaveConnectorResource(resource *sdk.ConnectorResource) error

	// GetConnectorResource 获取连接器资源定义
	GetConnectorResource(resourceID string) (*sdk.ConnectorResource, error)

	// ListConnectorResources 列出所有连接器资源定义
	ListConnectorResources() ([]*sdk.ConnectorResource, error)

	// DeleteConnectorResource 删除连接器资源定义
	DeleteConnectorResource(resourceID string) error

	// ==================== Watch 操作 ====================

	// WatchRules 监听规则变更
	// onUpdate: 当规则更新时调用
	// onDelete: 当规则删除时调用
	// 返回取消函数
	WatchRules(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) (context.CancelFunc, error)

	// WatchConnectorResources 监听连接器资源变更
	// onUpdate: 当连接器资源新增或更新时调用
	// onDelete: 当连接器资源删除时调用
	WatchConnectorResources(onUpdate func(resourceID string, data []byte) error, onDelete func(resourceID string) error) (context.CancelFunc, error)

	// ==================== 快照操作 ====================

	// SaveSnapshot 保存规则快照
	SaveSnapshot(snapshot *schema.RuleSnapshot) error

	// GetSnapshot 获取指定版本快照
	GetSnapshot(ruleID string, version int) (*schema.RuleSnapshot, error)

	// ListSnapshots 列出规则的所有快照
	ListSnapshots(ruleID string) ([]*schema.RuleSnapshot, error)

	// GetLatestSnapshot 获取最新快照
	GetLatestSnapshot(ruleID string) (*schema.RuleSnapshot, error)

	// ==================== 灰度发布 ====================

	// SaveGrayReleaseConfig 保存灰度发布配置
	SaveGrayReleaseConfig(config *schema.GrayReleaseConfig) error

	// GetGrayReleaseConfig 获取灰度发布配置
	GetGrayReleaseConfig(ruleID string) (*schema.GrayReleaseConfig, error)

	// DeleteGrayReleaseConfig 删除灰度发布配置
	DeleteGrayReleaseConfig(ruleID string) error
}

// 确保 SQLiteDB 和 PostgresDB 实现了 RulesDB 接口
var (
	_ RulesDB = (*SQLiteDB)(nil)
	_ RulesDB = (*PostgresDB)(nil)
)
