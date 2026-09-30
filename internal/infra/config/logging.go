package config

import pkglog "github.com/punk-one/iot-rule-engine/pkg/log"

// LoggingConfig 日志配置。
type LoggingConfig = pkglog.Config

func applyLoggingDefaults(cfg *Config) {
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.MaxSize <= 0 {
		cfg.Logging.MaxSize = 100
	}
	if cfg.Logging.MaxFiles <= 0 {
		cfg.Logging.MaxFiles = 5
	}
	if cfg.Logging.MaxBackups < 0 {
		cfg.Logging.MaxBackups = 0
	}
	if cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxBackups = 3
	}
	if cfg.Logging.PluginDir == "" {
		cfg.Logging.PluginDir = "./logs/rules"
	}
	if cfg.Logging.PluginMaxSize <= 0 {
		cfg.Logging.PluginMaxSize = cfg.Logging.MaxSize
	}
	if cfg.Logging.PluginMaxFiles <= 0 {
		cfg.Logging.PluginMaxFiles = cfg.Logging.MaxFiles
	}
	if cfg.Logging.PluginMaxBackups < 0 {
		cfg.Logging.PluginMaxBackups = 0
	}
	if cfg.Logging.PluginMaxBackups == 0 {
		cfg.Logging.PluginMaxBackups = cfg.Logging.MaxBackups
	}
}
