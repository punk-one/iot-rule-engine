package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
)

func TestEngineRunWaitsForCleanServiceShutdown(t *testing.T) {
	t.Setenv("IOT_RULE_ADMIN_PASSWORD", "Admin@Test123")
	dataDir := t.TempDir()
	cfg := &config.Config{
		Common: config.CommonConfig{NATS: config.NATSConfig{
			Mode:     config.NATSModeInternal,
			Host:     "127.0.0.1",
			Port:     0,
			StoreDir: filepath.Join(dataDir, "nats"),
		}},
		Manager: config.ManagerConfig{Port: 0, DBPath: dataDir, StorageType: "sqlite"},
		Executor: config.ExecutorConfig{
			PluginsDir:   filepath.Join(dataDir, "plugins"),
			WorkerCount:  1,
			StateBackend: config.StateBackendConfig{Type: "memory"},
		},
		Security: config.SecurityConfig{Manager: config.ManagerSecurityConfig{AuthEnabled: true}},
	}

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- engine.Run(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := bus.ActiveNATSOptions(); ok {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("engine did not initialize NATS")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("run engine: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("engine did not wait for and complete service shutdown")
	}
}
