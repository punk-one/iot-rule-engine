package config

// ExecutorConfig 执行器服务配置
type ExecutorConfig struct {
	PluginsDir            string             `yaml:"plugins_dir" json:"plugins_dir"`
	WorkerCount           int                `yaml:"worker_count" json:"worker_count"`
	NodeErrorHistoryLimit int                `yaml:"node_error_history_limit" json:"node_error_history_limit"`
	StateBackend          StateBackendConfig `yaml:"state_backend" json:"state_backend"`
	Delivery              DeliveryConfig     `yaml:"delivery" json:"delivery"` // 事件投递配置
	DBPath                string             `yaml:"db_path" json:"db_path"`   // 规则数据库路径
}

// StateBackendConfig Stateful 插件状态后端配置
// 用于 ChangeDetect、WindowAggregate 等有状态插件的状态缓存
// 注意：这与规则存储（manager.storage_type）是两个不同的配置
// 当 type=redis 时，使用全局 redis 配置
type StateBackendConfig struct {
	Type string `yaml:"type" json:"type"` // memory | redis
}

// DeliveryConfig 事件投递配置
type DeliveryConfig struct {
	Mode           string `yaml:"mode" json:"mode"`                       // best_effort | reliable
	EventTimeout   string `yaml:"event_timeout" json:"event_timeout"`     // 事件超时时间（如 "5s"）
	SerialDelivery bool   `yaml:"serial_delivery" json:"serial_delivery"` // 是否串行投递（默认 true）
}

// PipelineConfig 流水线服务配置
type PipelineConfig struct {
	DBPath string `yaml:"db_path" json:"db_path"` // 规则数据库路径
}
