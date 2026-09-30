package config

import "testing"

func validTestConfig() *Config {
	return &Config{
		Manager:  ManagerConfig{Port: 11011, StorageType: "sqlite"},
		Executor: ExecutorConfig{WorkerCount: 1, NodeErrorHistoryLimit: 100, StateBackend: StateBackendConfig{Type: "memory"}},
		Common:   CommonConfig{NATS: NATSConfig{Mode: NATSModeInternal}},
		Security: SecurityConfig{Manager: ManagerSecurityConfig{AuthEnabled: true}},
	}
}

func TestNormalizeAndValidateRejectsInvalidPersistentConfig(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"manager port", func(cfg *Config) { cfg.Manager.Port = 70000 }},
		{"storage", func(cfg *Config) { cfg.Manager.StorageType = "unknown" }},
		{"nats mode", func(cfg *Config) { cfg.Common.NATS.Mode = "unknown" }},
		{"overflow", func(cfg *Config) { cfg.Security.Dispatcher.OverflowStrategy = "discard" }},
		{"redis db", func(cfg *Config) {
			cfg.Executor.StateBackend.Type = "redis"
			cfg.Redis = RedisConfig{Host: "127.0.0.1", Port: 6379, DB: 16}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validTestConfig()
			test.edit(cfg)
			if err := NormalizeAndValidate(cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNormalizeAndValidateExternalNATSAuth(t *testing.T) {
	t.Setenv("IOT_RULE_NATS_PASSWORD", "")
	cfg := validTestConfig()
	cfg.Common.NATS = NATSConfig{Mode: NATSModeExternal, URL: "nats://localhost:4222", AuthMode: "user", Username: "iot", Password: "secret"}
	if err := NormalizeAndValidate(cfg); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
	cfg.Common.NATS.Password = ""
	if err := NormalizeAndValidate(cfg); err == nil {
		t.Fatal("expected missing password to fail")
	}
}

func TestNormalizeAndValidateAcceptsEnvironmentBackedNATSSecret(t *testing.T) {
	t.Setenv("IOT_RULE_NATS_PASSWORD", "environment-secret")
	cfg := validTestConfig()
	cfg.Common.NATS = NATSConfig{Mode: NATSModeExternal, URL: "nats://localhost:4222", AuthMode: "user", Username: "iot"}
	if err := NormalizeAndValidate(cfg); err != nil {
		t.Fatalf("expected environment-backed secret to validate: %v", err)
	}
	if cfg.Common.NATS.Password != "" {
		t.Fatal("validation must not copy environment secrets into persistent config")
	}
}
