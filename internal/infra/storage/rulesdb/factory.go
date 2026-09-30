package rulesdb

import (
	"fmt"
)

// StorageType 存储类型
type StorageType string

const (
	StorageTypeSQLite   StorageType = "sqlite"
	StorageTypePostgres StorageType = "postgres"
)

// OpenConfig 扩展的数据库配置
type OpenConfig struct {
	Config      // 嵌入基础配置
	StorageType StorageType
}

// OpenByType 根据配置打开数据库（工厂函数）
// storageType: sqlite | postgres
func OpenByType(cfg Config, storageType string) (RulesDB, error) {
	switch StorageType(storageType) {
	case StorageTypeSQLite, "":
		// 默认使用 SQLite
		return OpenSQLite(cfg)
	case StorageTypePostgres:
		// 检查 DatabaseConfig
		if cfg.DatabaseConfig == nil {
			return nil, fmt.Errorf("database config is required for postgres storage type")
		}
		// 类型断言在 OpenPostgres 中进行，避免循环导入
		return OpenPostgres(cfg, cfg.DatabaseConfig)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s (supported: sqlite, postgres)", storageType)
	}
}
