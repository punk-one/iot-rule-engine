package pipeline

import (
	"context"
	"encoding/json"

	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/infra/storage/rulesdb"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

func Run(ctx context.Context, cfg *config.Config) {
	// DB 应该由 Engine 创建并注入，这里不应该打开
	// 为了向后兼容，如果调用者没有提供 DB，则创建一个（但不推荐）
	pkglog.Warn("Run called without DB injection, creating temporary DB (not recommended)")
	dbPath := cfg.Pipeline.DBPath
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

	RunWithDB(ctx, cfg, db)
}

// RunWithDB 使用注入的 DB 运行 Pipeline
func RunWithDB(ctx context.Context, cfg *config.Config, db rulesdb.RulesDB) {
	// Create Pipeline Manager
	pm := NewManager()
	defer func() {
		if err := pm.Stop(); err != nil {
			pkglog.Warn("Failed to stop Pipeline Manager subscription: %v", err)
		}
	}()
	unregisterLocalManager := registerLocalManager(pm)
	defer unregisterLocalManager()

	// Pull all rules on startup
	pkglog.Info("Loading all rule configurations from database on startup...")
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
		if err := pm.UpdateRuleFromJSON(data); err != nil {
			preview := string(data)
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			pkglog.Error("Failed to load rule %s: %v (data length: %d, preview: %s)",
				rule.ID, err, len(data), preview)
			// 继续处理其他规则，不中断启动流程
		}
	}

	// Watch rules for changes（使用 DB Watch 替代 KV Watch）
	watchCancel, err := db.WatchRules(
		func(ruleID string, data []byte) error {
			pkglog.Info("Received config update for rule: %s", ruleID)
			if err := pm.UpdateRuleFromJSON(data); err != nil {
				preview := string(data)
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}
				pkglog.Error("Failed to update rule %s: %v (data length: %d, preview: %s)",
					ruleID, err, len(data), preview)
				// 不返回错误，继续处理其他规则，避免单个无效规则阻塞整个 watcher
			}
			return nil
		},
		func(ruleID string) error {
			pkglog.Info("Received config delete for rule: %s", ruleID)
			pm.UnregisterRule(ruleID)
			return nil
		},
	)
	if err != nil {
		pkglog.Error("Failed to watch rules: %v", err)
		return
	}
	defer watchCancel() // 在 context 取消时停止 watch
	// DB 由 Engine 管理，这里不关闭

	// Start Pipeline Manager
	if err := pm.Start(); err != nil {
		pkglog.Error("Failed to start pipeline manager: %v", err)
		return
	}

	pkglog.Info("Pipeline Manager started, routing data.ingest.* to rule.input.*")
	pkglog.Info("Pipeline Manager API merged into Manager server: /api/v1/pipeline/*")

	pkglog.Info("Pipeline Manager is running.")

	<-ctx.Done()

	pkglog.Info("Shutting down Pipeline Manager...")

	// Stop internal manager logic if needed?
	// pm.Stop() // logic if exists? pkg/pipeline/manager.go check?
	// Assuming pm doesn't have explicit Stop that blocks or needs call, or it relies on context.
	// The original main.go didn't call Stop on pm.

	pkglog.Info("Pipeline Manager shutdown complete")
}
