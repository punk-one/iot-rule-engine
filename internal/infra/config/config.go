package config

// Config 全局配置
type Config struct {
	Common   CommonConfig   `yaml:"common" json:"common"`
	Manager  ManagerConfig  `yaml:"manager" json:"manager"`
	Executor ExecutorConfig `yaml:"executor" json:"executor"`
	Pipeline PipelineConfig `yaml:"pipeline" json:"pipeline"`
	Security SecurityConfig `yaml:"security" json:"security"` // 安全配置
	Database DatabaseConfig `yaml:"database" json:"database"` // PostgreSQL 数据库配置
	Redis    RedisConfig    `yaml:"redis" json:"redis"`       // Redis 配置（用于 Stateful Plugin）
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`   // 日志配置
	PProf    PProfConfig    `yaml:"pprof" json:"pprof"`       // pprof 性能分析配置
}
