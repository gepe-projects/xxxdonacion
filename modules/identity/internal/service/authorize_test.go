package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/cache"
	"xxxdonacion/platform/apperr"
)

func newTestCache(t *testing.T) (*cache.UserCache, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	return cache.NewUserCache(rdb), rdb
}

func TestAuthorizeUsesCacheFirst(t *testing.T) {
	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, "uid-1", domain.User{
		AuthID: "uid-1",
		Email:  "admin@example.com",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleAdmin},
	})

	// DB nil: test gagal kalau Authorize menyentuh database.
	s := New(nil, rdb, nil)
	user, err := s.Authorize(ctx, "uid-1", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if user.Email != "admin@example.com" {
		t.Fatalf("user = %+v, want cached admin", user)
	}
}

func TestAuthorizeFallsBackToDatabaseOnCacheMiss(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if !strings.Contains(sql, "WHERE auth_id = $1") {
				return fakeRow{err: pgx.ErrNoRows}
			}
			return fakeRow{values: []any{
				uuid.NewV7(),
				pgtype.Text{String: "uid-1", Valid: true},
				"admin@example.com",
				pgtype.Text{String: "Admin", Valid: true},
				string(domain.StatusActive),
				now,
				now,
			}}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{}, nil
			}
			return &fakeRows{data: [][]any{{"ADMIN"}}}, nil
		},
	}

	userCache, rdb := newTestCache(t)
	s := New(db, rdb, nil)

	user, err := s.Authorize(context.Background(), "uid-1", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if user.Email != "admin@example.com" {
		t.Fatalf("user = %+v, want admin", user)
	}
	if _, ok := userCache.Get(context.Background(), "uid-1"); !ok {
		t.Fatal("cache was not populated after DB fallback")
	}
}

func TestAuthorizeRejectsDisabledAndMissingRole(t *testing.T) {
	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, "uid-1", domain.User{
		AuthID: "uid-1",
		Status: domain.StatusDisabled,
		Roles:  []domain.Role{domain.RoleUser},
	})
	s := New(nil, rdb, nil)

	if _, err := s.Authorize(ctx, "uid-1"); err == nil {
		t.Fatal("expected disabled error")
	}

	userCache.Set(ctx, "uid-2", domain.User{
		AuthID: "uid-2",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleUser},
	})
	_, err := s.Authorize(ctx, "uid-2", domain.RoleAdmin)
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindForbidden {
		t.Fatalf("error = %v, want forbidden", err)
	}
}

func TestAuthorizeReturnsNotFoundForUnprovisionedUser(t *testing.T) {
	db := &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: pgx.ErrNoRows}
		},
	}
	userCache, rdb := newTestCache(t)
	s := New(db, rdb, nil)

	_, err := s.Authorize(context.Background(), "uid-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := userCache.Get(context.Background(), "uid-1"); ok {
		t.Fatal("unprovisioned user should not be cached")
	}
}
