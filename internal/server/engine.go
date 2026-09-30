package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/infra/pipeline"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	"github.com/punk-one/iot-rule-engine/internal/server/executor"
	"github.com/punk-one/iot-rule-engine/internal/server/manager"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// Engine 顶层引擎，负责统一管理数据库和服务生命周期
// 作为 rules.db 的唯一 Owner
type Engine struct {
	db         rulesdb.RulesDB
	cfg        *config.Config
	runtimeMgr *runtime.Manager
}

type supervisedService struct {
	name string
	run  func(context.Context) error
}

type serviceResult struct {
	name string
	err  error
}

// NewEngine 创建新的 Engine 实例
func NewEngine(cfg *config.Config) (*Engine, error) {
	// 确定数据库路径
	dbPath := cfg.Manager.DBPath
	if dbPath == "" {
		dbPath = "data"
	}

	// 打开数据库（唯一 Owner，自动选择存储类型）
	db, err := rulesdb.Open(rulesdb.Config{
		DataDir:        dbPath,
		StorageType:    cfg.Manager.StorageType,
		DatabaseConfig: &cfg.Database,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open rules.db: %w", err)
	}

	// 创建 Runtime Manager
	runtimeMgr := runtime.NewManagerWithOptions(runtime.ManagerOptions{
		MaxNodeErrorHistory: cfg.Executor.NodeErrorHistoryLimit,
	})

	return &Engine{
		db:         db,
		cfg:        cfg,
		runtimeMgr: runtimeMgr,
	}, nil
}

// Run 启动所有服务（串行初始化，避免并发打开数据库）
// 规范：bus.Init() 只能由 Engine 调用
func (e *Engine) Run(ctx context.Context) error {
	// 初始化消息总线（必须在所有服务启动前）
	// 规范要求：bus.Init() 只能由 Engine 调用
	natsOpts := bus.NATSOptions{
		Mode:          e.cfg.Common.NATS.Mode,
		Host:          e.cfg.Common.NATS.Host,
		Port:          e.cfg.Common.NATS.Port,
		StoreDir:      e.cfg.Common.NATS.StoreDir,
		URL:           e.cfg.Common.NATS.URL,
		AuthMode:      e.cfg.Common.NATS.AuthMode,
		MaxReconnects: e.cfg.Common.NATS.MaxReconnects,
		ReconnectWait: time.Duration(e.cfg.Common.NATS.ReconnectWaitMS) * time.Millisecond,
		Username:      e.cfg.Common.NATS.Username,
		Password:      e.cfg.Common.NATS.Password,
		Token:         e.cfg.Common.NATS.Token,
		CertFile:      e.cfg.Common.NATS.CertFile,
		KeyFile:       e.cfg.Common.NATS.KeyFile,
		CAFile:        e.cfg.Common.NATS.CAFile,
		CredsFile:     e.cfg.Common.NATS.CredsFile,
	}
	if err := bus.Init(natsOpts); err != nil {
		return fmt.Errorf("failed to initialize bus: %w", err)
	}
	activeNATS, ok := bus.ActiveNATSOptions()
	if !ok {
		bus.Close()
		return fmt.Errorf("failed to resolve active NATS connection")
	}
	// Plugins connect through their own clients, so they need the actual random
	// loopback URL chosen by the embedded server.
	e.cfg.Common.NATS.URL = activeNATS.URL
	e.cfg.Common.NATS.AuthMode = activeNATS.AuthMode
	e.cfg.Common.NATS.Username = activeNATS.Username
	e.cfg.Common.NATS.Password = activeNATS.Password
	e.cfg.Common.NATS.Token = activeNATS.Token
	e.cfg.Common.NATS.CertFile = activeNATS.CertFile
	e.cfg.Common.NATS.KeyFile = activeNATS.KeyFile
	e.cfg.Common.NATS.CAFile = activeNATS.CAFile
	e.cfg.Common.NATS.CredsFile = activeNATS.CredsFile

	pluginsDir := e.cfg.Executor.PluginsDir
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}
	pluginLoader := plugin.NewLoader(pluginsDir)
	pluginLoader.SetRestartPolicy(plugin.RestartPolicy{
		BaseDelay:        time.Duration(e.cfg.Security.Plugin.RestartBaseDelayMs) * time.Millisecond,
		MaxDelay:         time.Duration(e.cfg.Security.Plugin.RestartMaxDelayMs) * time.Millisecond,
		MaxRestarts:      e.cfg.Security.Plugin.RestartMaxRetries,
		JitterRatio:      e.cfg.Security.Plugin.RestartJitterRatio,
		FailureThreshold: e.cfg.Security.Plugin.RestartFailureThreshold,
		CircuitCooldown:  time.Duration(e.cfg.Security.Plugin.RestartCircuitCooldownMs) * time.Millisecond,
	})
	if err := pluginLoader.ScanPlugins(); err != nil {
		pkglog.Error("Failed to scan plugins: %v", err)
	}
	defer pluginLoader.Close()

	services := []supervisedService{
		{
			name: "manager",
			run: func(runCtx context.Context) error {
				return manager.RunWithRuntimeAndDBEAndLoader(runCtx, e.cfg, e.runtimeMgr, e.db, pluginLoader)
			},
		},
		{
			name: "executor",
			run: func(runCtx context.Context) error {
				executor.RunWithRuntimeAndDBAndLoader(runCtx, e.cfg, e.runtimeMgr, e.db, pluginLoader)
				return nil
			},
		},
		{
			name: "pipeline",
			run: func(runCtx context.Context) error {
				pipeline.RunWithDB(runCtx, e.cfg, e.db)
				return nil
			},
		},
	}

	pkglog.Info("Engine started all supervised services")
	return superviseServices(ctx, services)
}

func superviseServices(ctx context.Context, services []supervisedService) error {
	if len(services) == 0 {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan serviceResult, len(services))
	for _, service := range services {
		service := service
		go func() {
			err := service.run(runCtx)
			if err == nil && runCtx.Err() == nil {
				err = fmt.Errorf("service exited unexpectedly")
			}
			results <- serviceResult{name: service.name, err: err}
		}()
	}

	collected := 0
	var result error
	select {
	case first := <-results:
		collected++
		if first.err != nil {
			result = fmt.Errorf("%s: %w", first.name, first.err)
		}
		cancel()
	case <-ctx.Done():
		cancel()
	}

	for collected < len(services) {
		serviceResult := <-results
		collected++
		if serviceResult.err == nil || errors.Is(serviceResult.err, context.Canceled) {
			continue
		}
		result = errors.Join(result, fmt.Errorf("%s: %w", serviceResult.name, serviceResult.err))
	}
	return result
}

// Close 关闭 Engine 和数据库
// 规范：bus.Close() 只能由 Engine 调用
func (e *Engine) Close() error {
	// 关闭消息总线（必须在所有服务关闭后）
	bus.Close()

	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

// GetDB 获取数据库实例（用于高级操作）
func (e *Engine) GetDB() rulesdb.RulesDB {
	return e.db
}

// GetDBPath 获取数据库路径（用于配置加载等场景）
func (e *Engine) GetDBPath() string {
	if e.cfg.Manager.DBPath != "" {
		return filepath.Join(e.cfg.Manager.DBPath, "rules.db")
	}
	return "data/rules.db"
}
