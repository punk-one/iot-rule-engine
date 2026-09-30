package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

var openRulesDB = rulesdb.Open

// Load 加载配置
// 优先级：1. data/rules.db 中的配置 2. config.yaml 的默认值
// path 参数为空时读取默认配置文件，否则读取指定配置文件
func Load(path string) (*Config, error) {
	configPath := path
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	// 1. 先从默认 YAML 加载基础配置（包括 StorageType）
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read default config %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal default config: %w", err)
	}

	// 2. 尝试从数据库读取配置并覆盖
	tryLoadConfigFromDB(&cfg, data)
	applyEnvironmentSecrets(&cfg)

	if err := NormalizeAndValidate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// applyEnvironmentSecrets runs after database/YAML loading so deployment
// secrets always win and never need to be persisted in repository config.
func applyEnvironmentSecrets(cfg *Config) {
	apply := func(name string, target *string) {
		if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
			*target = value
		}
	}
	apply("IOT_RULE_DATABASE_PASSWORD", &cfg.Database.Password)
	apply("IOT_RULE_REDIS_PASSWORD", &cfg.Redis.Password)
	apply("IOT_RULE_NATS_USERNAME", &cfg.Common.NATS.Username)
	apply("IOT_RULE_NATS_PASSWORD", &cfg.Common.NATS.Password)
	apply("IOT_RULE_NATS_TOKEN", &cfg.Common.NATS.Token)
	apply("IOT_RULE_NATS_CREDS_FILE", &cfg.Common.NATS.CredsFile)
	apply("IOT_RULE_NATS_CERT_FILE", &cfg.Common.NATS.CertFile)
	apply("IOT_RULE_NATS_KEY_FILE", &cfg.Common.NATS.KeyFile)
	apply("IOT_RULE_NATS_CA_FILE", &cfg.Common.NATS.CAFile)
	apply("IOT_RULE_MANAGER_TOKEN", &cfg.Security.Manager.Token)
}

func applyNATSDefaults(cfg *Config) error {
	natsCfg := &cfg.Common.NATS
	natsCfg.Mode = strings.ToLower(strings.TrimSpace(natsCfg.Mode))
	if natsCfg.Mode == "" {
		natsCfg.Mode = NATSModeInternal
	}
	if natsCfg.MaxReconnects == 0 {
		natsCfg.MaxReconnects = -1
	}
	if natsCfg.ReconnectWaitMS <= 0 {
		natsCfg.ReconnectWaitMS = 2000
	}

	switch natsCfg.Mode {
	case NATSModeInternal:
		if natsCfg.Port < 0 || natsCfg.Port > 65535 {
			return fmt.Errorf("internal port must be between 0 and 65535, got %d", natsCfg.Port)
		}
		if strings.TrimSpace(natsCfg.Host) == "" {
			natsCfg.Host = "127.0.0.1"
		}
		if strings.TrimSpace(natsCfg.StoreDir) == "" {
			dataDir := cfg.Manager.DBPath
			if strings.TrimSpace(dataDir) == "" {
				dataDir = "data"
			}
			natsCfg.StoreDir = filepath.Join(dataDir, "nats")
		}

		// The embedded server is process-local and intentionally has no credentials.
		natsCfg.URL = ""
		natsCfg.AuthMode = "none"
		natsCfg.Username = ""
		natsCfg.Password = ""
		natsCfg.Token = ""
		natsCfg.CertFile = ""
		natsCfg.KeyFile = ""
		natsCfg.CAFile = ""
		natsCfg.CredsFile = ""
	case NATSModeExternal:
		if strings.TrimSpace(natsCfg.URL) == "" {
			return fmt.Errorf("external mode requires url")
		}
		natsCfg.AuthMode = strings.ToLower(strings.TrimSpace(natsCfg.AuthMode))
		if natsCfg.AuthMode == "" {
			natsCfg.AuthMode = "none"
		}
	default:
		return fmt.Errorf("mode must be %q or %q, got %q", NATSModeInternal, NATSModeExternal, natsCfg.Mode)
	}
	return nil
}

func tryLoadConfigFromDB(cfg *Config, yamlData []byte) {
	dbPath := cfg.Manager.DBPath
	if dbPath == "" {
		dbPath = "data"
	}

	storageType := cfg.Manager.StorageType
	if storageType == "" {
		storageType = string(rulesdb.StorageTypeSQLite)
	}

	db, err := openRulesDB(rulesdb.Config{
		DataDir:        dbPath,
		StorageType:    storageType,
		DatabaseConfig: &cfg.Database,
	})
	if err != nil {
		pkglog.Warn("Failed to open database to load config, using YAML defaults: %v", err)
		return
	}
	defer db.Close()

	shouldSeedConfig := storageType != string(rulesdb.StorageTypePostgres) || cfg.Manager.InitDB
	if shouldSeedConfig {
		exists, err := db.ConfigExists()
		if err != nil {
			pkglog.Warn("Failed to check persisted config state, using YAML/default behavior: %v", err)
		} else if !exists {
			if err := db.SaveConfig(yamlData); err != nil {
				pkglog.Warn("Failed to initialize database config from YAML: %v", err)
			} else {
				pkglog.Info("Initialized database config from YAML because the system config row was empty")
			}
		} else {
			pkglog.Info("System config already exists in database, skipping YAML seed")
		}
	}

	configData, err := db.GetConfig()
	if err != nil {
		if isConfigNotFoundErr(err) {
			pkglog.Info("System config not found in database, using YAML defaults")
			return
		}
		pkglog.Warn("Failed to read config from database, using YAML defaults: %v", err)
		return
	}
	if len(configData) == 0 {
		return
	}

	if err := yaml.Unmarshal(configData, cfg); err != nil {
		pkglog.Warn("Failed to unmarshal config from database, using YAML defaults: %v", err)
		return
	}

	if storageType == string(rulesdb.StorageTypePostgres) {
		pkglog.Info("Loaded configuration from postgres database")
		return
	}
	pkglog.Info("Loaded configuration from database: %s", filepath.Join(dbPath, "rules.db"))
}

func isConfigNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "config not found")
}
