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
	"xxxdonacion/platform/postgres"
	"xxxdonacion/platform/redis"
	"xxxdonacion/platform/server"
)

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	return serve(cfg)
}

func serve(cfg *config.App) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := postgres.New(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	slog.Info("postgres connected")

	rdb, err := redis.New(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	slog.Info("redis connected")

	server := server.New(cfg)

	// wiring module
	identityModule := identity.New(identity.Dependencies{DB: pool, Redis: rdb})

	// register routes
	identityModule.RegisterRoutes(server.Echo)

	slog.Info("http server listening", "addr", cfg.Port, "env", cfg.Environment)

	return server.Start()
}

func loadConfig() (*config.App, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return config.Load()
}
