package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
)

type service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

// Foo implements [domain.Service].
func (s *service) Foo(ctx context.Context) (string, error) {
	if err := s.db.Ping(ctx); err != nil {
		slog.Error("failed pring db", "err", err)
		return "", fmt.Errorf("opps something went wrong")
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		slog.Error("failed pring redis", "err", err)
		return "", fmt.Errorf("opps something went wrong")
	}

	return "Bar", nil
}

func New(db *pgxpool.Pool,
	rdb *redis.Client,
) domain.Service {
	// setup sqlc/repository

	return &service{
		db:  db,
		rdb: rdb,
	}
}
