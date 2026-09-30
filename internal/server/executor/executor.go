package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/panjf2000/ants/v2"
	"github.com/punk-one/iot-rule-engine/internal/core/engine"
	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
	"github.com/punk-one/iot-rule-engine/pkg/state"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

func Run(ctx context.Context, cfg *config.Config) {
	RunWithRuntime(ctx, cfg, runtime.NewManager())
}

func RunWithRuntime(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager) {
	// Plugin Loader
	pluginsDir := cfg.Executor.PluginsDir
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}

	// Create Plugin Loader and Scan
	pluginLoader := plugin.NewLoader(pluginsDir)
	pluginLoader.SetRestartPolicy(plugin.RestartPolicy{
		BaseDelay:        time.Duration(cfg.Security.Plugin.RestartBaseDelayMs) * time.Millisecond,
		MaxDelay:         time.Duration(cfg.Security.Plugin.RestartMaxDelayMs) * time.Millisecond,
		MaxRestarts:      cfg.Security.Plugin.RestartMaxRetries,
		JitterRatio:      cfg.Security.Plugin.RestartJitterRatio,
		FailureThreshold: cfg.Security.Plugin.RestartFailureThreshold,
		CircuitCooldown:  time.Duration(cfg.Security.Plugin.RestartCircuitCooldownMs) * time.Millisecond,
	})
	if err := pluginLoader.ScanPlugins(); err != nil {
		pkglog.Error("Failed to scan plugins: %v", err)
	}

	// Runtime Manager is now passed in

	// Create State Manager（支持全局配置）
	stateBackend, err := state.NewBackendFromConfig(cfg.Executor.StateBackend, cfg)
	if err != nil {
		pkglog.Error("Failed to create state backend: %v", err)
		return
	}
	defer stateBackend.Close()
	stateMgr := state.NewStateManager(stateBackend)
	defer stateMgr.Close()

	// Create Executor
	executor, err := engine.NewExecutorWithLoader(pluginLoader, runtimeMgr)
	if err != nil {
		pkglog.Error("Failed to create executor: %v", err)
		return
	}
	defer executor.Close()
	defer pluginLoader.Close()
	unregisterLocalExecutor := registerLocalExecutor(executor)
	defer unregisterLocalExecutor()

	// Set State Manager to Executor
	executor.SetStateManager(stateMgr)
	executor.SetPluginRuntimeDefaults(engine.PluginRuntimeDefaults{
		BufferDBPath: resolveBufferDBPath(cfg),
		NATS: bus.NATSOptions{
			Mode:          cfg.Common.NATS.Mode,
			Host:          cfg.Common.NATS.Host,
			Port:          cfg.Common.NATS.Port,
			StoreDir:      cfg.Common.NATS.StoreDir,
			URL:           cfg.Common.NATS.URL,
			AuthMode:      cfg.Common.NATS.AuthMode,
			MaxReconnects: cfg.Common.NATS.MaxReconnects,
			ReconnectWait: time.Duration(cfg.Common.NATS.ReconnectWaitMS) * time.Millisecond,
			Username:      cfg.Common.NATS.Username,
			Password:      cfg.Common.NATS.Password,
			Token:         cfg.Common.NATS.Token,
			CertFile:      cfg.Common.NATS.CertFile,
			KeyFile:       cfg.Common.NATS.KeyFile,
			CAFile:        cfg.Common.NATS.CAFile,
			CredsFile:     cfg.Common.NATS.CredsFile,
		},
	})
	executor.SetDispatcherSecurityConfig(&engine.DispatcherSecurityConfig{
		MaxQueue:              cfg.Security.Dispatcher.MaxQueue,
		RuleQPS:               cfg.Security.Dispatcher.RuleQPS,
		NodeQPS:               cfg.Security.Dispatcher.NodeQPS,
		DeduplicationWindowMs: cfg.Security.Dispatcher.DeduplicationWindowMs,
		MaxPayloadKB:          cfg.Security.Plugin.MaxPayloadKB,
		OverflowStrategy:      cfg.Security.Dispatcher.OverflowStrategy,
	})

	// DB 应该由 Engine 创建并注入，这里不应该打开
	// 为了向后兼容，如果调用者没有提供 DB，则创建一个（但不推荐）
	pkglog.Warn("RunWithRuntime called without DB injection, creating temporary DB (not recommended)")
	dbPath := cfg.Executor.DBPath
	if dbPath == "" {
		dbPath = cfg.Manager.DBPath
		if dbPath == "" {
			dbPath = "data"
		}
	}
	db, err := rulesdb.Open(rulesdb.Config{
		DataDir:        dbPath,
		StorageType:    cfg.Manager.StorageType,
		DatabaseConfig: &cfg.Database,
	})
	if err != nil {
		pkglog.Error("Failed to initialize database: %v", err)
		return
	}
	defer db.Close()

	RunWithRuntimeAndDB(ctx, cfg, runtimeMgr, db)
}

// RunWithRuntimeAndDB 使用指定的 RuntimeManager 和 DB 运行 Executor
func RunWithRuntimeAndDB(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB) {
	RunWithRuntimeAndDBAndLoader(ctx, cfg, runtimeMgr, db, nil)
}

// RunWithRuntimeAndDBAndLoader reuses an engine-owned loader when provided.
// Standalone callers retain ownership of the loader created here.
func RunWithRuntimeAndDBAndLoader(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB, sharedLoader *plugin.Loader) {
	// Plugin Loader
	pluginsDir := cfg.Executor.PluginsDir
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}

	pluginLoader := sharedLoader
	ownLoader := pluginLoader == nil
	if ownLoader {
		pluginLoader = plugin.NewLoader(pluginsDir)
		pluginLoader.SetRestartPolicy(plugin.RestartPolicy{
			BaseDelay:        time.Duration(cfg.Security.Plugin.RestartBaseDelayMs) * time.Millisecond,
			MaxDelay:         time.Duration(cfg.Security.Plugin.RestartMaxDelayMs) * time.Millisecond,
			MaxRestarts:      cfg.Security.Plugin.RestartMaxRetries,
			JitterRatio:      cfg.Security.Plugin.RestartJitterRatio,
			FailureThreshold: cfg.Security.Plugin.RestartFailureThreshold,
			CircuitCooldown:  time.Duration(cfg.Security.Plugin.RestartCircuitCooldownMs) * time.Millisecond,
		})
		if err := pluginLoader.ScanPlugins(); err != nil {
			pkglog.Error("Failed to scan plugins: %v", err)
		}
	}

	// Create State Manager（支持全局配置）
	stateBackend, err := state.NewBackendFromConfig(cfg.Executor.StateBackend, cfg)
	if err != nil {
		pkglog.Error("Failed to create state backend: %v", err)
		return
	}
	defer stateBackend.Close()
	stateMgr := state.NewStateManager(stateBackend)
	defer stateMgr.Close()

	// Create Executor
	executor, err := engine.NewExecutorWithLoader(pluginLoader, runtimeMgr)
	if err != nil {
		pkglog.Error("Failed to create executor: %v", err)
		return
	}
	defer executor.Close()
	if ownLoader {
		defer pluginLoader.Close()
	}
	unregisterLocalExecutor := registerLocalExecutor(executor)
	defer unregisterLocalExecutor()

	// Set State Manager to Executor
	executor.SetStateManager(stateMgr)
	executor.SetPluginRuntimeDefaults(engine.PluginRuntimeDefaults{
		BufferDBPath: resolveBufferDBPath(cfg),
		NATS: bus.NATSOptions{
			Mode:          cfg.Common.NATS.Mode,
			Host:          cfg.Common.NATS.Host,
			Port:          cfg.Common.NATS.Port,
			StoreDir:      cfg.Common.NATS.StoreDir,
			URL:           cfg.Common.NATS.URL,
			AuthMode:      cfg.Common.NATS.AuthMode,
			MaxReconnects: cfg.Common.NATS.MaxReconnects,
			ReconnectWait: time.Duration(cfg.Common.NATS.ReconnectWaitMS) * time.Millisecond,
			Username:      cfg.Common.NATS.Username,
			Password:      cfg.Common.NATS.Password,
			Token:         cfg.Common.NATS.Token,
			CertFile:      cfg.Common.NATS.CertFile,
			KeyFile:       cfg.Common.NATS.KeyFile,
			CAFile:        cfg.Common.NATS.CAFile,
			CredsFile:     cfg.Common.NATS.CredsFile,
		},
	})
	executor.SetDispatcherSecurityConfig(&engine.DispatcherSecurityConfig{
		MaxQueue:              cfg.Security.Dispatcher.MaxQueue,
		RuleQPS:               cfg.Security.Dispatcher.RuleQPS,
		NodeQPS:               cfg.Security.Dispatcher.NodeQPS,
		DeduplicationWindowMs: cfg.Security.Dispatcher.DeduplicationWindowMs,
		MaxPayloadKB:          cfg.Security.Plugin.MaxPayloadKB,
		OverflowStrategy:      cfg.Security.Dispatcher.OverflowStrategy,
	})

	// Load connector resources before loading rules so connector_ref bindings can resolve.
	connectorResources, err := db.ListConnectorResources()
	if err != nil {
		pkglog.Error("Failed to list connector resources: %v", err)
		return
	}
	for _, resource := range connectorResources {
		if resource == nil {
			continue
		}
		if err := executor.RegisterConnectorResource(*resource); err != nil {
			pkglog.Error("Failed to register connector resource %s: %v", resource.ID, err)
		}
	}
	pkglog.Info("Loaded %d connector resources on startup", len(connectorResources))

	connectorWatchCancel, err := db.WatchConnectorResources(
		func(resourceID string, data []byte) error {
			var resource sdk.ConnectorResource
			if err := json.Unmarshal(data, &resource); err != nil {
				return fmt.Errorf("failed to decode connector resource %s: %w", resourceID, err)
			}
			resource.ID = resourceID
			pkglog.Info("Received connector resource update: %s", resourceID)
			return executor.RegisterConnectorResource(resource)
		},
		func(resourceID string) error {
			pkglog.Info("Received connector resource delete: %s", resourceID)
			return executor.DeleteConnectorResource(resourceID)
		},
	)
	if err != nil {
		pkglog.Error("Failed to watch connector resources: %v", err)
		return
	}
	defer connectorWatchCancel()

	// Pull all rules on startup
	pkglog.Info("Loading all rules from database on startup...")
	rules, err := db.ListRules()
	if err != nil {
		pkglog.Error("Failed to list rules: %v", err)
		return
	}

	for _, rule := range rules {
		data, err := json.Marshal(rule)
		if err != nil {
			pkglog.Error("Failed to marshal rule %s: %v", rule.ID, err)
			continue
		}

		pkglog.Info("Loading rule from DB: %s", rule.ID)
		if err := executor.UpdateDAGFromJSON(data); err != nil {
			preview := string(data)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			pkglog.Error("Failed to load rule %s: %v (data length: %d, preview: %s)",
				rule.ID, err, len(data), preview)
			// 继续处理其他规则，不中断启动流程
		}
	}
	pkglog.Info("Loaded %d rules on startup", len(executor.ListRules()))

	// Watch rules for changes（使用 DB Watch 替代 KV Watch）
	watchCancel, err := db.WatchRules(
		func(ruleID string, data []byte) error {
			pkglog.Info("Received config update for rule: %s", ruleID)
			if err := executor.UpdateDAGFromJSON(data); err != nil {
				preview := string(data)
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}
				pkglog.Error("Failed to update DAG for rule %s: %v (data length: %d, preview: %s)",
					ruleID, err, len(data), preview)
				// 不返回错误，继续处理其他规则，避免单个无效规则阻塞整个 watcher
			} else {
			}
			return nil
		},
		func(ruleID string) error {
			pkglog.Info("Received config delete for rule: %s", ruleID)
			executor.UnregisterDAG(ruleID)
			return nil
		},
	)
	if err != nil {
		pkglog.Error("Failed to watch rules: %v", err)
		return
	}
	defer watchCancel() // 在 context 取消时停止 watch

	// Worker Pool
	workerCount := cfg.Executor.WorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}
	pool, err := ants.NewPool(workerCount)
	if err != nil {
		pkglog.Error("Failed to create worker pool: %v", err)
		return
	}
	defer pool.Release()

	// Subscribe（使用语义级 API）
	queueGroup := "executor-group"

	subscription, err := bus.SubscribeAllRuleInputsManualAckManaged(queueGroup, func(msg *nats.Msg) {
		// Submit to pool
		err := pool.Submit(func() {
			// Process
			var ruleMsg schema.RuleMessage
			if err := json.Unmarshal(msg.Data, &ruleMsg); err != nil {
				pkglog.Error("Failed to unmarshal message: %v", err)
				msg.Nak()
				return
			}

			// Using distinct timeout for execution
			execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			if err := executor.Execute(execCtx, &ruleMsg); err != nil {
				pkglog.Error("Failed to execute rule %s: %v", ruleMsg.RuleID, err)
				msg.Nak()
				return
			}

			msg.Ack()
		})

		if err != nil {
			pkglog.Error("Failed to submit task to pool: %v", err)
			msg.Nak()
		}
	})
	if err != nil {
		pkglog.Error("Failed to subscribe: %v", err)
		return
	}
	defer func() {
		if err := subscription.Close(); err != nil {
			pkglog.Warn("Failed to close executor subscription: %v", err)
		}
	}()

	pkglog.Info("Executor started, listening on rule.input.> with queue group %s (Workers: %d)", queueGroup, workerCount)

	// Runtime trace persistence is written directly by RuntimeObserver.
	// sys.trace.* remains a debug side channel and is not mirrored back here.

	pkglog.Info("Executor API merged into Manager server: /api/v1/executor/* and /api/v1/runtime/*")

	pkglog.Info("Executor is running.")

	<-ctx.Done()

	pkglog.Info("Shutting down Executor...")

	pkglog.Info("Executor shutdown complete")
}

func resolveBufferDBPath(cfg *config.Config) string {
	if cfg == nil {
		return "data/buffer.db"
	}

	dataDir := cfg.Executor.DBPath
	if dataDir == "" {
		dataDir = cfg.Manager.DBPath
	}
	if dataDir == "" {
		dataDir = "data"
	}
	return filepath.Join(dataDir, "buffer.db")
}
