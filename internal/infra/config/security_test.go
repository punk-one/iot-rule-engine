package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSecurityDefaultsFailClosed(t *testing.T) {
	cfg := &Config{}
	applySecurityDefaults(cfg)
	if !cfg.Security.Manager.AuthEnabled {
		t.Fatal("manager authentication must be enabled by default")
	}
	if cfg.Security.Manager.MaxPluginUploadMB != 64 {
		t.Fatalf("unexpected upload limit: %d", cfg.Security.Manager.MaxPluginUploadMB)
	}
	if cfg.Security.Manager.AllowRuntimePluginInstall {
		t.Fatal("runtime plugin installation must be disabled by default")
	}
}

func TestSecurityDefaultsAllowExplicitInsecureDevelopmentMode(t *testing.T) {
	cfg := &Config{Security: SecurityConfig{Manager: ManagerSecurityConfig{AllowInsecure: true}}}
	applySecurityDefaults(cfg)
	if cfg.Security.Manager.AuthEnabled {
		t.Fatal("explicit insecure development mode should remain disabled")
	}
}

func TestConfigJSONNeverExposesDeploymentSecrets(t *testing.T) {
	cfg := Config{
		Common:   CommonConfig{NATS: NATSConfig{Password: "nats-password", Token: "nats-token"}},
		Database: DatabaseConfig{Password: "database-password"},
		Redis:    RedisConfig{Password: "redis-password"},
		Security: SecurityConfig{Manager: ManagerSecurityConfig{Token: "manager-token"}},
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	for _, secret := range []string{"nats-password", "nats-token", "database-password", "redis-password", "manager-token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("JSON exposed secret %q: %s", secret, encoded)
		}
	}
}
