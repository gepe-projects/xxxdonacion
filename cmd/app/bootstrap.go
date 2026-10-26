package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"

	"xxxdonacion/modules/identity"
	"xxxdonacion/platform/config"
	"xxxdonacion/platform/firebase"
	"xxxdonacion/platform/logger"
	"xxxdonacion/platform/postgres"
	"xxxdonacion/platform/redis"
	"xxxdonacion/platform/server"
)

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(cfg)

	return serve(cfg, log)
}

func serve(cfg *config.App, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	log.Info("postgres connected", "component", "postgres")

	rdb, err := redis.New(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	log.Info("redis connected", "component", "redis")

	fb, err := firebase.NewClient(ctx, &cfg.Firebase)
	if err != nil {
		return fmt.Errorf("firebase: %w", err)
	}
	log.Info("firebase connected", "component", "firebase")

	srv := server.New(cfg)

	// wiring module
	identityModule := identity.New(identity.Dependencies{
		DB:       pool,
		Redis:    rdb,
		Firebase: fb,
	})

	// register routes
	identityModule.RegisterRoutes(srv.Echo)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Info("http server starting", "component", "http", "addr", addr)

	if err := srv.Start(); err != nil {
		return fmt.Errorf("http server: %w", err)
	}

	log.Info("http server stopped", "component", "http")
	return nil
}

func loadConfig() (*config.App, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return config.Load()
}
