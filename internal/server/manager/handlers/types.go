package handlers

import (
	"sync"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	"github.com/punk-one/iot-rule-plugin-sdk"
)

// Manager 管理规则和插件
type Manager struct {
	db            rulesdb.RulesDB
	pluginLoader  *plugin.Loader
	pluginsDir    string
	executorURL   string
	runtimeMgr    *runtime.Manager
	installMu     sync.Mutex
	installPolicy PluginInstallPolicy
}

type PluginInstallPolicy struct {
	Enabled                bool
	TrustedManifestPath    string
	MaxUploadBytes         int64
	RegistryURL            string
	RegistryToken          string
	CosignPath             string
	SigstoreIssuer         string
	SigstoreIdentityRegexp string
}

// NewManager 创建新的 Manager
func NewManager(db rulesdb.RulesDB, pluginsDir string, executorURL string, runtimeMgr *runtime.Manager) *Manager {
	return NewManagerWithLoader(db, pluginsDir, executorURL, runtimeMgr, nil)
}

// NewManagerWithLoader creates a manager backed by the process-wide plugin
// loader. A nil loader preserves the standalone/test behavior.
func NewManagerWithLoader(db rulesdb.RulesDB, pluginsDir string, executorURL string, runtimeMgr *runtime.Manager, loader *plugin.Loader) *Manager {
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}
	if loader == nil {
		loader = plugin.NewLoader(pluginsDir)
		loader.ScanPlugins() // 扫描插件（可选）
	}

	return &Manager{
		db:           db,
		pluginLoader: loader,
		pluginsDir:   pluginsDir,
		executorURL:  executorURL,
		runtimeMgr:   runtimeMgr,
	}
}

func (m *Manager) SetPluginInstallPolicy(policy PluginInstallPolicy) {
	if policy.MaxUploadBytes <= 0 {
		policy.MaxUploadBytes = 64 << 20
	}
	if policy.CosignPath == "" {
		policy.CosignPath = "cosign"
	}
	if policy.SigstoreIssuer == "" {
		policy.SigstoreIssuer = "https://token.actions.githubusercontent.com"
	}
	if policy.SigstoreIdentityRegexp == "" {
		policy.SigstoreIdentityRegexp = `^https://github\.com/punk-one/iot-rule-plugins/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
	}
	m.installPolicy = policy
}

// RuleSummary 规则摘要
type RuleSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     int       `json:"version"`
	Enabled     bool      `json:"enabled"`
	DebugMode   bool      `json:"debug_mode"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	Status      string    `json:"status"`
	NodeCount   int       `json:"node_count"`
}

// PluginMeta 插件元数据
type PluginMeta struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	EngineAPI   string          `json:"engine_api,omitempty"`
	UploadedAt  string          `json:"uploaded_at,omitempty"`
	Info        *sdk.PluginInfo `json:"info,omitempty"`
}

// ValidationError 验证错误
type ValidationError struct {
	NodeID  string `json:"node_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ValidationWarning 验证警告
type ValidationWarning struct {
	Message string `json:"message"`
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid    bool                `json:"valid"`
	Errors   []ValidationError   `json:"errors"`
	Warnings []ValidationWarning `json:"warnings"`
}
