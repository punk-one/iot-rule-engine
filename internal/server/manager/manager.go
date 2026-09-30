package manager

import (
	"context"

	"github.com/punk-one/iot-rule-engine/internal/core/plugin"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// Run 启动 Manager 服务
func Run(ctx context.Context, cfg *config.Config) {
	RunWithRuntime(ctx, cfg, runtime.NewManager())
}

// RunWithRuntime 使用指定的 RuntimeManager 启动服务
func RunWithRuntime(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager) {
	dbPath := cfg.Manager.DBPath
	if dbPath == "" {
		dbPath = "data"
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

// RunWithRuntimeAndDB 使用指定的 RuntimeManager 和 DB 启动服务
func RunWithRuntimeAndDB(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB) {
	if err := RunWithRuntimeAndDBE(ctx, cfg, runtimeMgr, db); err != nil {
		pkglog.Error("Manager stopped with error: %v", err)
	}
}

// RunWithRuntimeAndDBE is the supervised variant used by the top-level engine.
func RunWithRuntimeAndDBE(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB) error {
	return RunWithRuntimeAndDBEAndLoader(ctx, cfg, runtimeMgr, db, nil)
}

// RunWithRuntimeAndDBEAndLoader is used by the unified engine to reuse its
// process-wide plugin loader.
func RunWithRuntimeAndDBEAndLoader(ctx context.Context, cfg *config.Config, runtimeMgr *runtime.Manager, db rulesdb.RulesDB, pluginLoader *plugin.Loader) error {
	return RunWithServerEAndLoader(ctx, cfg, runtimeMgr, db, pluginLoader)
}
