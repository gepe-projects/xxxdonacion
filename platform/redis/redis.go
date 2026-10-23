package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"xxxdonacion/platform/config"
)

func New(ctx context.Context, cfg config.Redis) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping: %w", err)
	}
	return client, nil
}
