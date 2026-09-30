package config

// ManagerConfig 规则管理服务配置
type ManagerConfig struct {
	Port        int    `yaml:"port" json:"port"`
	DBPath      string `yaml:"db_path" json:"db_path"`           // 数据目录（存储 rules.db）
	InitDB      bool   `yaml:"init_db" json:"init_db"`           // PostgreSQL 首次启动时初始化 system 配置
	StorageType string `yaml:"storage_type" json:"storage_type"` // sqlite | postgres
}
