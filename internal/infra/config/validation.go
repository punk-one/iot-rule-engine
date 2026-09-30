package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// NormalizeAndValidate applies the same defaults used during startup and then
// validates cross-field constraints. It is used by both startup loading and the
// Manager API so an invalid configuration cannot be persisted for the next boot.
func NormalizeAndValidate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}
	if cfg.Manager.Port == 0 {
		cfg.Manager.Port = 11011
	}
	if cfg.Executor.WorkerCount == 0 {
		cfg.Executor.WorkerCount = 1
	}
	if cfg.Executor.NodeErrorHistoryLimit == 0 {
		cfg.Executor.NodeErrorHistoryLimit = 100
	}
	if err := applyNATSDefaults(cfg); err != nil {
		return fmt.Errorf("common.nats: %w", err)
	}
	applySecurityDefaults(cfg)
	applyLoggingDefaults(cfg)

	if cfg.Manager.Port < 1 || cfg.Manager.Port > 65535 {
		return fmt.Errorf("manager.port must be between 1 and 65535")
	}
	storageType := strings.ToLower(strings.TrimSpace(cfg.Manager.StorageType))
	if storageType == "" {
		storageType = "sqlite"
	}
	if storageType != "sqlite" && storageType != "postgres" {
		return fmt.Errorf("manager.storage_type must be sqlite or postgres")
	}
	cfg.Manager.StorageType = storageType
	if storageType == "postgres" {
		if strings.TrimSpace(cfg.Database.Host) == "" || strings.TrimSpace(cfg.Database.Name) == "" || strings.TrimSpace(cfg.Database.User) == "" {
			return fmt.Errorf("database.host, database.name and database.user are required for postgres")
		}
		if cfg.Database.Port < 1 || cfg.Database.Port > 65535 {
			return fmt.Errorf("database.port must be between 1 and 65535")
		}
	}
	if cfg.Executor.WorkerCount < 1 {
		return fmt.Errorf("executor.worker_count must be greater than zero")
	}
	if cfg.Executor.NodeErrorHistoryLimit < 1 {
		return fmt.Errorf("executor.node_error_history_limit must be greater than zero")
	}
	stateType := strings.ToLower(strings.TrimSpace(cfg.Executor.StateBackend.Type))
	if stateType == "" {
		stateType = "memory"
	}
	if stateType != "memory" && stateType != "redis" {
		return fmt.Errorf("executor.state_backend.type must be memory or redis")
	}
	cfg.Executor.StateBackend.Type = stateType
	if stateType == "redis" {
		if strings.TrimSpace(cfg.Redis.Host) == "" || cfg.Redis.Port < 1 || cfg.Redis.Port > 65535 {
			return fmt.Errorf("redis.host and a valid redis.port are required for redis state")
		}
		if cfg.Redis.DB < 0 || cfg.Redis.DB > 15 {
			return fmt.Errorf("redis.db must be between 0 and 15")
		}
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Executor.Delivery.Mode))
	if mode == "" {
		mode = "best_effort"
	}
	if mode != "best_effort" && mode != "reliable" {
		return fmt.Errorf("executor.delivery.mode must be best_effort or reliable")
	}
	cfg.Executor.Delivery.Mode = mode
	if strings.TrimSpace(cfg.Executor.Delivery.EventTimeout) != "" {
		duration, err := time.ParseDuration(cfg.Executor.Delivery.EventTimeout)
		if err != nil || duration <= 0 {
			return fmt.Errorf("executor.delivery.event_timeout must be a positive duration")
		}
	}
	if cfg.Common.NATS.Mode == NATSModeExternal {
		parsed, err := url.Parse(cfg.Common.NATS.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("common.nats.url must be an absolute URL in external mode")
		}
		authMode := strings.ToLower(strings.TrimSpace(cfg.Common.NATS.AuthMode))
		switch authMode {
		case "none":
		case "user":
			if strings.TrimSpace(cfg.Common.NATS.Username) == "" || !configuredSecret(cfg.Common.NATS.Password, "IOT_RULE_NATS_PASSWORD") {
				return fmt.Errorf("common.nats username and password are required for user auth")
			}
		case "token":
			if !configuredSecret(cfg.Common.NATS.Token, "IOT_RULE_NATS_TOKEN") {
				return fmt.Errorf("common.nats token is required for token auth")
			}
		case "tls":
			if cfg.Common.NATS.CertFile == "" || cfg.Common.NATS.KeyFile == "" || cfg.Common.NATS.CAFile == "" {
				return fmt.Errorf("common.nats cert_file, key_file and ca_file are required for tls auth")
			}
		case "creds":
			if cfg.Common.NATS.CredsFile == "" {
				return fmt.Errorf("common.nats creds_file is required for creds auth")
			}
		default:
			return fmt.Errorf("common.nats.auth_mode must be none, user, token, tls or creds")
		}
	}
	switch cfg.Security.Dispatcher.OverflowStrategy {
	case "reject", "drop_oldest", "block":
	default:
		return fmt.Errorf("security.dispatcher.overflow_strategy must be reject, drop_oldest or block")
	}
	if !cfg.Security.Manager.AuthEnabled && !cfg.Security.Manager.AllowInsecure {
		return fmt.Errorf("security.manager requires auth_enabled or explicit allow_insecure")
	}
	return nil
}

func configuredSecret(persisted, environmentName string) bool {
	if strings.TrimSpace(persisted) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv(environmentName)) != ""
}
