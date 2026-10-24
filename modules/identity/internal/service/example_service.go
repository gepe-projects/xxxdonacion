package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/platform/apperr"
)

type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func New(db *pgxpool.Pool,
	rdb *redis.Client,
) *Service {
	// setup sqlc/repository

	return &Service{
		db:  db,
		rdb: rdb,
	}
}

// Foo implements [domain.Service].
func (s *Service) Foo(ctx context.Context) (string, error) {
	if err := s.db.Ping(ctx); err != nil {
		return "", apperr.Wrap(
			fmt.Errorf("ping postgres: %w", err),
			apperr.KindInternal,
			"oops something went wrong",
		)
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		return "", apperr.Wrap(
			fmt.Errorf("ping redis: %w", err),
			apperr.KindInternal,
			"oops something went wrong",
		)
	}

	return "Bar", nil
}

func (s *Service) InternalError(ctx context.Context) error {
	return apperr.Wrap(
		errors.New("pala bapak kau anjing"),
		apperr.KindInternal,
		"oops something went wrong",
	)
}
