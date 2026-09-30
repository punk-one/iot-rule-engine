package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func TestLoadInitializesPostgresConfigWhenInitDBEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := []byte(`manager:
  port: 10101
  storage_type: "postgres"
  init_db: true
  db_path: "./data"
database:
  host: "127.0.0.1"
  port: 5432
  name: "punk"
  user: "punk"
  password: "secret"
  ssl_mode: "disable"
`)
	if err := os.WriteFile(configPath, yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	oldOpen := openRulesDB
	defer func() { openRulesDB = oldOpen }()

	fakeDB := &fakeRulesDB{configErr: errors.New("config not found")}
	openRulesDB = func(cfg rulesdb.Config) (rulesdb.RulesDB, error) {
		return fakeDB, nil
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !fakeDB.saveCalled {
		t.Fatal("expected SaveConfig to be called")
	}
	if string(fakeDB.savedConfig) != string(yamlContent) {
		t.Fatalf("expected saved config to match yaml\nwant: %s\ngot:  %s", string(yamlContent), string(fakeDB.savedConfig))
	}
	if cfg.Manager.Port != 10101 {
		t.Fatalf("expected manager port 10101, got %d", cfg.Manager.Port)
	}
}

func TestLoadDoesNotOverwriteExistingPostgresConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := []byte(`manager:
  port: 10101
  storage_type: "postgres"
  init_db: true
  db_path: "./data"
`)
	if err := os.WriteFile(configPath, yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	dbConfig := []byte(`manager:
  port: 20202
  storage_type: "postgres"
  init_db: true
  db_path: "./db-from-db"
database:
  host: "127.0.0.1"
  port: 5432
  name: "punk"
  user: "punk"
`)

	oldOpen := openRulesDB
	defer func() { openRulesDB = oldOpen }()

	fakeDB := &fakeRulesDB{configExists: true, configData: dbConfig}
	openRulesDB = func(cfg rulesdb.Config) (rulesdb.RulesDB, error) {
		return fakeDB, nil
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if fakeDB.saveCalled {
		t.Fatal("did not expect SaveConfig to be called when config already exists")
	}
	if cfg.Manager.Port != 20202 {
		t.Fatalf("expected config from db, got manager port %d", cfg.Manager.Port)
	}
	if cfg.Manager.DBPath != "./db-from-db" {
		t.Fatalf("expected db path from db, got %q", cfg.Manager.DBPath)
	}
}

func TestLoadDoesNotInitializePostgresConfigWhenInitDBDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := []byte(`manager:
  port: 10101
  storage_type: "postgres"
  init_db: false
  db_path: "./data"
database:
  host: "127.0.0.1"
  port: 5432
  name: "punk"
  user: "punk"
`)
	if err := os.WriteFile(configPath, yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	oldOpen := openRulesDB
	defer func() { openRulesDB = oldOpen }()

	fakeDB := &fakeRulesDB{configErr: errors.New("config not found")}
	openRulesDB = func(cfg rulesdb.Config) (rulesdb.RulesDB, error) {
		return fakeDB, nil
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if fakeDB.saveCalled {
		t.Fatal("did not expect SaveConfig to be called")
	}
	if cfg.Manager.Port != 10101 {
		t.Fatalf("expected yaml config to remain active, got manager port %d", cfg.Manager.Port)
	}
}

func TestLoadSeedsSQLiteConfigOnFirstStart(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := []byte(`manager:
  port: 10101
  storage_type: "sqlite"
  db_path: "./data"
`)
	if err := os.WriteFile(configPath, yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	oldOpen := openRulesDB
	defer func() { openRulesDB = oldOpen }()

	fakeDB := &fakeRulesDB{configErr: errors.New("config not found")}
	openRulesDB = func(cfg rulesdb.Config) (rulesdb.RulesDB, error) {
		return fakeDB, nil
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !fakeDB.saveCalled {
		t.Fatal("expected first-start SQLite config to be persisted")
	}
	if string(fakeDB.savedConfig) != string(yamlContent) {
		t.Fatalf("persisted config does not match bootstrap YAML")
	}
	if cfg.Manager.StorageType != "sqlite" || cfg.Manager.Port != 10101 {
		t.Fatalf("unexpected loaded config: %+v", cfg.Manager)
	}
}

func TestEnvironmentSecretsOverridePersistedConfiguration(t *testing.T) {
	t.Setenv("IOT_RULE_DATABASE_PASSWORD", "db-from-env")
	t.Setenv("IOT_RULE_REDIS_PASSWORD", "redis-from-env")
	t.Setenv("IOT_RULE_MANAGER_TOKEN", "manager-from-env")

	cfg := &Config{
		Database: DatabaseConfig{Password: "persisted-db"},
		Redis:    RedisConfig{Password: "persisted-redis"},
		Security: SecurityConfig{Manager: ManagerSecurityConfig{Token: "persisted-manager"}},
	}
	applyEnvironmentSecrets(cfg)
	if cfg.Database.Password != "db-from-env" || cfg.Redis.Password != "redis-from-env" || cfg.Security.Manager.Token != "manager-from-env" {
		t.Fatalf("environment secrets were not applied: %+v", cfg)
	}
}

func TestLoadDefaultsManagerPortTo11011(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := []byte(`manager:
  storage_type: "sqlite"
  db_path: "./data"
`)
	if err := os.WriteFile(configPath, yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	oldOpen := openRulesDB
	defer func() { openRulesDB = oldOpen }()
	openRulesDB = func(cfg rulesdb.Config) (rulesdb.RulesDB, error) {
		return &fakeRulesDB{configErr: errors.New("config not found")}, nil
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Manager.Port != 11011 {
		t.Fatalf("expected manager port 11011, got %d", cfg.Manager.Port)
	}
}

type fakeRulesDB struct {
	configExists bool
	configData   []byte
	configErr    error
	saveCalled   bool
	savedConfig  []byte
}

func (f *fakeRulesDB) Close() error                                    { return nil }
func (f *fakeRulesDB) Path() string                                    { return "" }
func (f *fakeRulesDB) SaveRule(ruleFlow *schema.RuleFlow) error        { return nil }
func (f *fakeRulesDB) GetRule(ruleID string) (*schema.RuleFlow, error) { return nil, nil }
func (f *fakeRulesDB) ListRules() ([]*schema.RuleFlow, error)          { return nil, nil }
func (f *fakeRulesDB) DeleteRule(ruleID string) error                  { return nil }
func (f *fakeRulesDB) RuleExists(ruleID string) (bool, error)          { return false, nil }
func (f *fakeRulesDB) SaveConfig(configData []byte) error {
	f.saveCalled = true
	f.savedConfig = append([]byte(nil), configData...)
	f.configExists = true
	f.configData = append([]byte(nil), configData...)
	f.configErr = nil
	return nil
}
func (f *fakeRulesDB) GetConfig() ([]byte, error) {
	if f.configErr != nil {
		return nil, f.configErr
	}
	return append([]byte(nil), f.configData...), nil
}
func (f *fakeRulesDB) ConfigExists() (bool, error)                                 { return f.configExists, nil }
func (f *fakeRulesDB) SaveConnectorResource(resource *sdk.ConnectorResource) error { return nil }
func (f *fakeRulesDB) GetConnectorResource(resourceID string) (*sdk.ConnectorResource, error) {
	return nil, nil
}
func (f *fakeRulesDB) ListConnectorResources() ([]*sdk.ConnectorResource, error) { return nil, nil }
func (f *fakeRulesDB) DeleteConnectorResource(resourceID string) error           { return nil }
func (f *fakeRulesDB) WatchRules(onUpdate func(ruleID string, data []byte) error, onDelete func(ruleID string) error) (context.CancelFunc, error) {
	return func() {}, nil
}
func (f *fakeRulesDB) WatchConnectorResources(onUpdate func(resourceID string, data []byte) error, onDelete func(resourceID string) error) (context.CancelFunc, error) {
	return func() {}, nil
}
func (f *fakeRulesDB) SaveSnapshot(snapshot *schema.RuleSnapshot) error { return nil }
func (f *fakeRulesDB) GetSnapshot(ruleID string, version int) (*schema.RuleSnapshot, error) {
	return nil, nil
}
func (f *fakeRulesDB) ListSnapshots(ruleID string) ([]*schema.RuleSnapshot, error) { return nil, nil }
func (f *fakeRulesDB) GetLatestSnapshot(ruleID string) (*schema.RuleSnapshot, error) {
	return nil, nil
}
func (f *fakeRulesDB) SaveGrayReleaseConfig(config *schema.GrayReleaseConfig) error { return nil }
func (f *fakeRulesDB) GetGrayReleaseConfig(ruleID string) (*schema.GrayReleaseConfig, error) {
	return nil, nil
}
func (f *fakeRulesDB) DeleteGrayReleaseConfig(ruleID string) error { return nil }

func TestApplyNATSDefaultsUsesInternalRandomPort(t *testing.T) {
	cfg := Config{}
	cfg.Manager.DBPath = "./state"
	cfg.Common.NATS = NATSConfig{
		URL:       "nats://legacy.example:4222",
		AuthMode:  "creds",
		Password:  "stale-password",
		CredsFile: "stale.creds",
	}

	if err := applyNATSDefaults(&cfg); err != nil {
		t.Fatalf("applyNATSDefaults returned error: %v", err)
	}
	got := cfg.Common.NATS
	if got.Mode != NATSModeInternal {
		t.Fatalf("expected internal mode, got %q", got.Mode)
	}
	if got.Host != "127.0.0.1" || got.Port != 0 {
		t.Fatalf("unexpected internal listener: %s:%d", got.Host, got.Port)
	}
	if got.StoreDir != filepath.Join("state", "nats") {
		t.Fatalf("unexpected store dir: %q", got.StoreDir)
	}
	if got.URL != "" || got.AuthMode != "none" || got.Password != "" || got.CredsFile != "" {
		t.Fatalf("internal mode retained external connection credentials: %+v", got)
	}
}

func TestApplyNATSDefaultsRequiresExternalURL(t *testing.T) {
	cfg := Config{Common: CommonConfig{NATS: NATSConfig{Mode: NATSModeExternal}}}
	if err := applyNATSDefaults(&cfg); err == nil {
		t.Fatal("expected external mode without URL to fail")
	}
}

func TestApplyNATSDefaultsRejectsInvalidInternalPort(t *testing.T) {
	cfg := Config{Common: CommonConfig{NATS: NATSConfig{
		Mode: NATSModeInternal,
		Port: 65536,
	}}}
	if err := applyNATSDefaults(&cfg); err == nil {
		t.Fatal("expected invalid internal port to fail")
	}
}
