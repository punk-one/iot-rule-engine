package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/punk-one/iot-rule-engine/internal/infra/config"
	"github.com/punk-one/iot-rule-engine/internal/server"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if err := pkglog.Init(cfg.Logging); err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eng, err := server.NewEngine(cfg)
	if err != nil {
		return fmt.Errorf("create engine: %w", err)
	}
	defer eng.Close()

	if err := eng.Run(ctx); err != nil {
		return fmt.Errorf("run engine: %w", err)
	}

	pkglog.Info("Shutting down all services...")
	pkglog.Info("All services stopped.")
	return nil
}

func loadConfig() (*config.Config, error) {
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}
