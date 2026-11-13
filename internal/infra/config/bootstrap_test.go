package config

import (
	"path/filepath"
	"testing"
)

func TestFileIndependentBootstrapUsesIsolatedDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("IOT_RULE_DATA_DIR", root)
	t.Setenv("IOT_RULE_LISTEN", "127.0.0.1:12091")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Manager.DBPath != root || cfg.Manager.Port != 12091 || cfg.Manager.Host != "127.0.0.1" || cfg.Executor.PluginsDir != filepath.Join(root, "plugins") {
		t.Fatalf("bootstrap overrides not applied: %+v", cfg.Manager)
	}
	second, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if second.Manager.DBPath != root {
		t.Fatal("persisted configuration relocated instance")
	}
}
