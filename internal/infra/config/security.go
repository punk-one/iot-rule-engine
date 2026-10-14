package config

// SecurityConfig 安全配置
type SecurityConfig struct {
	Plugin     PluginSecurityConfig     `yaml:"plugin" json:"plugin"`
	Dispatcher DispatcherSecurityConfig `yaml:"dispatcher" json:"dispatcher"`
	Manager    ManagerSecurityConfig    `yaml:"manager" json:"manager"`
}

// PluginSecurityConfig 插件安全配置
type PluginSecurityConfig struct {
	MaxEmitQPS               int     `yaml:"max_emit_qps" json:"max_emit_qps"`                               // Emit QPS 限制
	RPCTimeoutMs             int     `yaml:"rpc_timeout_ms" json:"rpc_timeout_ms"`                           // RPC 超时（毫秒）
	MaxPayloadKB             int     `yaml:"max_payload_kb" json:"max_payload_kb"`                           // Payload 最大大小（KB）
	RestartBaseDelayMs       int     `yaml:"restart_base_delay_ms" json:"restart_base_delay_ms"`             // 插件重启基础延迟
	RestartMaxDelayMs        int     `yaml:"restart_max_delay_ms" json:"restart_max_delay_ms"`               // 插件重启最大延迟
	RestartMaxRetries        int     `yaml:"restart_max_retries" json:"restart_max_retries"`                 // 插件重启最大重试次数
	RestartJitterRatio       float64 `yaml:"restart_jitter_ratio" json:"restart_jitter_ratio"`               // 插件重启抖动比例
	RestartFailureThreshold  int     `yaml:"restart_failure_threshold" json:"restart_failure_threshold"`     // 熔断阈值
	RestartCircuitCooldownMs int     `yaml:"restart_circuit_cooldown_ms" json:"restart_circuit_cooldown_ms"` // 熔断冷却时间
}

// DispatcherSecurityConfig Dispatcher 安全配置
type DispatcherSecurityConfig struct {
	MaxQueue              int    `yaml:"max_queue" json:"max_queue"`                             // 队列最大大小
	RuleQPS               int    `yaml:"rule_qps" json:"rule_qps"`                               // Rule 级 QPS
	NodeQPS               int    `yaml:"node_qps" json:"node_qps"`                               // Node 级 QPS
	DeduplicationWindowMs int    `yaml:"deduplication_window_ms" json:"deduplication_window_ms"` // 去重时间窗口（毫秒）
	OverflowStrategy      string `yaml:"overflow_strategy" json:"overflow_strategy"`             // reject | drop_oldest | block
}

// ManagerSecurityConfig Manager 安全配置
type ManagerSecurityConfig struct {
	Token                     string   `yaml:"token" json:"-"` // 兼容旧版静态 Token；建议使用用户会话
	AuthEnabled               bool     `yaml:"auth_enabled" json:"auth_enabled"`
	AllowInsecure             bool     `yaml:"allow_insecure" json:"allow_insecure"`
	AllowedOrigins            []string `yaml:"allowed_origins" json:"allowed_origins"`
	AllowRuntimePluginInstall bool     `yaml:"allow_runtime_plugin_install" json:"allow_runtime_plugin_install"`
	TrustedPluginManifest     string   `yaml:"trusted_plugin_manifest" json:"trusted_plugin_manifest"`
	MaxPluginUploadMB         int64    `yaml:"max_plugin_upload_mb" json:"max_plugin_upload_mb"`
	PluginRegistryURL         string   `yaml:"plugin_registry_url" json:"plugin_registry_url"`
	PluginRegistryTokenEnv    string   `yaml:"plugin_registry_token_env" json:"plugin_registry_token_env"`
	CosignPath                string   `yaml:"cosign_path" json:"cosign_path"`
	SigstoreIssuer            string   `yaml:"sigstore_issuer" json:"sigstore_issuer"`
	SigstoreIdentityRegexp    string   `yaml:"sigstore_identity_regexp" json:"sigstore_identity_regexp"`
}

// applySecurityDefaults 应用安全配置默认值
func applySecurityDefaults(cfg *Config) {
	// Plugin 安全配置默认值
	if cfg.Security.Plugin.MaxEmitQPS <= 0 {
		cfg.Security.Plugin.MaxEmitQPS = 10000
	}
	if cfg.Security.Plugin.RPCTimeoutMs <= 0 {
		cfg.Security.Plugin.RPCTimeoutMs = 5000 // 5秒
	}
	if cfg.Security.Plugin.MaxPayloadKB <= 0 {
		cfg.Security.Plugin.MaxPayloadKB = 256
	}
	if cfg.Security.Plugin.RestartBaseDelayMs <= 0 {
		cfg.Security.Plugin.RestartBaseDelayMs = 2000
	}
	if cfg.Security.Plugin.RestartMaxDelayMs <= 0 {
		cfg.Security.Plugin.RestartMaxDelayMs = 30000
	}
	if cfg.Security.Plugin.RestartMaxRetries <= 0 {
		cfg.Security.Plugin.RestartMaxRetries = 5
	}
	if cfg.Security.Plugin.RestartJitterRatio <= 0 || cfg.Security.Plugin.RestartJitterRatio > 1 {
		cfg.Security.Plugin.RestartJitterRatio = 0.2
	}
	if cfg.Security.Plugin.RestartFailureThreshold <= 0 {
		cfg.Security.Plugin.RestartFailureThreshold = 5
	}
	if cfg.Security.Plugin.RestartCircuitCooldownMs <= 0 {
		cfg.Security.Plugin.RestartCircuitCooldownMs = 30000
	}

	// Dispatcher 安全配置默认值
	if cfg.Security.Dispatcher.MaxQueue <= 0 {
		cfg.Security.Dispatcher.MaxQueue = 100000
	}
	if cfg.Security.Dispatcher.RuleQPS <= 0 {
		cfg.Security.Dispatcher.RuleQPS = 10000
	}
	if cfg.Security.Dispatcher.NodeQPS <= 0 {
		cfg.Security.Dispatcher.NodeQPS = 1000
	}
	if cfg.Security.Dispatcher.DeduplicationWindowMs <= 0 {
		cfg.Security.Dispatcher.DeduplicationWindowMs = 1000 // 1秒
	}
	if cfg.Security.Dispatcher.OverflowStrategy == "" {
		cfg.Security.Dispatcher.OverflowStrategy = "reject"
	}

	// 默认必须鉴权。明确设置 allow_insecure 才能关闭，避免漏写配置时裸露控制面。
	if !cfg.Security.Manager.AuthEnabled && !cfg.Security.Manager.AllowInsecure {
		cfg.Security.Manager.AuthEnabled = true
	}
	if cfg.Security.Manager.MaxPluginUploadMB <= 0 {
		cfg.Security.Manager.MaxPluginUploadMB = 64
	}
	if cfg.Security.Manager.PluginRegistryTokenEnv == "" {
		cfg.Security.Manager.PluginRegistryTokenEnv = "IOT_RULE_PLUGIN_REGISTRY_TOKEN"
	}
	if cfg.Security.Manager.CosignPath == "" {
		cfg.Security.Manager.CosignPath = "cosign"
	}
	if cfg.Security.Manager.SigstoreIssuer == "" {
		cfg.Security.Manager.SigstoreIssuer = "https://token.actions.githubusercontent.com"
	}
	if cfg.Security.Manager.SigstoreIdentityRegexp == "" {
		cfg.Security.Manager.SigstoreIdentityRegexp = `^https://github\.com/punk-one/iot-rule-plugins/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
	}
}
