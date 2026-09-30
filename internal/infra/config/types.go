package config

const (
	NATSModeInternal = "internal"
	NATSModeExternal = "external"
)

// NATSConfig NATS 消息总线配置
type NATSConfig struct {
	Mode            string `yaml:"mode" json:"mode"`           // internal, external
	Host            string `yaml:"host" json:"host"`           // internal mode only
	Port            int    `yaml:"port" json:"port"`           // internal mode only; 0 selects a random port
	StoreDir        string `yaml:"store_dir" json:"store_dir"` // internal JetStream storage
	URL             string `yaml:"url" json:"url"`
	AuthMode        string `yaml:"auth_mode" json:"auth_mode"`           // none, user, token, tls, creds
	MaxReconnects   int    `yaml:"max_reconnects" json:"max_reconnects"` // -1 means retry forever
	ReconnectWaitMS int    `yaml:"reconnect_wait_ms" json:"reconnect_wait_ms"`
	Username        string `yaml:"username" json:"username"`
	Password        string `yaml:"password" json:"-"`
	Token           string `yaml:"token" json:"-"`
	CertFile        string `yaml:"cert_file" json:"cert_file"`
	KeyFile         string `yaml:"key_file" json:"key_file"`
	CAFile          string `yaml:"ca_file" json:"ca_file"`
	CredsFile       string `yaml:"creds_file" json:"creds_file"`
}

// CommonConfig 通用配置
type CommonConfig struct {
	NATS NATSConfig `yaml:"nats" json:"nats"`
}

// DatabaseConfig PostgreSQL 数据库配置
// 用于规则存储，manager.storage_type=postgres 时使用
type DatabaseConfig struct {
	Host                   string `yaml:"host" json:"host"`
	Port                   int    `yaml:"port" json:"port"`
	Name                   string `yaml:"name" json:"name"`
	User                   string `yaml:"user" json:"user"`
	Password               string `yaml:"password" json:"-"`
	SSLMode                string `yaml:"ssl_mode" json:"ssl_mode"`                                   // disable, require, verify-ca, verify-full
	MaxIdleConns           int    `yaml:"max_idle_conns" json:"max_idle_conns"`                       // 最大空闲连接数
	MaxOpenConns           int    `yaml:"max_open_conns" json:"max_open_conns"`                       // 最大打开连接数
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes" json:"conn_max_lifetime_minutes"` // 连接最大生命周期（分钟）
}

// PProfConfig pprof 性能分析配置
type PProfConfig struct {
	Enabled              bool `yaml:"enabled" json:"enabled"`
	BlockProfileRate     int  `yaml:"block_profile_rate" json:"block_profile_rate"`
	MutexProfileFraction int  `yaml:"mutex_profile_fraction" json:"mutex_profile_fraction"`
}

// RedisConfig Redis 配置
// 用于 Stateful 插件状态缓存，state_backend.type=redis 时使用
type RedisConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	Password string `yaml:"password" json:"-"`
	DB       int    `yaml:"db" json:"db"` // Redis 数据库编号（0-15）
}
