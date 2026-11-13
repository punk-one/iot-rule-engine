package config

import (
	_ "embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

//go:embed defaults.yaml
var defaultConfig []byte

func applyBootstrap(cfg *Config) error {
	if value := os.Getenv("IOT_RULE_PLUGIN_INSTALL_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid IOT_RULE_PLUGIN_INSTALL_ENABLED")
		}
		cfg.Security.Manager.AllowRuntimePluginInstall = enabled
	}
	if value := os.Getenv("IOT_RULE_COSIGN_PATH"); value != "" {
		cfg.Security.Manager.CosignPath = value
	}
	if mode := os.Getenv("IOT_RULE_DELIVERY_MODE"); mode != "" {
		if mode != "reliable" && mode != "best_effort" {
			return fmt.Errorf("invalid IOT_RULE_DELIVERY_MODE")
		}
		cfg.Executor.Delivery.Mode = mode
	}
	if root := os.Getenv("IOT_RULE_DATA_DIR"); root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		cfg.Manager.DBPath = abs
		cfg.Executor.DBPath = abs
		cfg.Pipeline.DBPath = abs
		cfg.Common.NATS.StoreDir = filepath.Join(abs, "nats")
		cfg.Logging.File = filepath.Join(abs, "logs", "engine.log")
		cfg.Logging.PluginDir = filepath.Join(abs, "logs", "rules")
		cfg.Executor.PluginsDir = filepath.Join(abs, "plugins")
	}
	if plugins := os.Getenv("IOT_RULE_PLUGINS_DIR"); plugins != "" {
		cfg.Executor.PluginsDir = plugins
	}
	if listen := os.Getenv("IOT_RULE_LISTEN"); listen != "" {
		host, port, err := net.SplitHostPort(listen)
		if err != nil {
			return fmt.Errorf("IOT_RULE_LISTEN: %w", err)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid IOT_RULE_LISTEN port")
		}
		cfg.Manager.Host, cfg.Manager.Port = host, n
	}
	return nil
}
