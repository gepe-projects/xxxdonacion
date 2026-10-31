package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func TestSyncRejectsUnverifiedEmail(t *testing.T) {
	s := New(nil, nil, nil)

	_, err := s.Sync(context.Background(), domain.Principal{
		UID:           "uid-1",
		Email:         "user@example.com",
		EmailVerified: false,
	})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != apperr.KindForbidden {
		t.Fatalf("kind = %v, want forbidden", appErr.Kind)
	}
}

func TestSyncRequiresUID(t *testing.T) {
	s := New(nil, nil, nil)

	_, err := s.Sync(context.Background(), domain.Principal{EmailVerified: true})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != apperr.KindUnauthorized {
		t.Fatalf("kind = %v, want unauthorized", appErr.Kind)
	}
}

func TestSyncInvalidatesCacheAfterProfileUpdate(t *testing.T) {
	const uid = "uid-sync-profile"
	userID := uuid.NewV7()

	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM identity.users") && strings.Contains(sql, "auth_id = $1"):
				return fakeRow{values: []any{
					userID,
					pgtype.Text{String: uid, Valid: true},
					"old@example.com",
					pgtype.Text{String: "Old Name", Valid: true},
					string(domain.StatusActive),
					pgtype.Timestamptz{},
					pgtype.Timestamptz{},
				}}
			case strings.Contains(sql, "UPDATE identity.users"):
				return fakeRow{values: []any{
					userID,
					pgtype.Text{String: uid, Valid: true},
					"new@example.com",
					pgtype.Text{String: "New Name", Valid: true},
					string(domain.StatusActive),
					pgtype.Timestamptz{},
					pgtype.Timestamptz{},
				}}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{data: [][]any{{string(domain.RoleAdmin)}}}, nil
			}
			return &fakeRows{}, nil
		},
	}

	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, uid, domain.User{
		AuthID: uid,
		Email:  "old@example.com",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleUser},
	})

	s := New(db, rdb, nil)
	user, err := s.Sync(ctx, domain.Principal{
		UID:           uid,
		Email:         "new@example.com",
		Name:          "New Name",
		EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if user.Email != "new@example.com" || user.Name != "New Name" {
		t.Fatalf("user = %+v, want updated profile", user)
	}
	if _, ok := userCache.Get(ctx, uid); ok {
		t.Fatal("cache hit after sync")
	}
}

func TestSyncInvalidatesCacheWithoutProfileChange(t *testing.T) {
	const uid = "uid-sync-noop"
	userID := uuid.NewV7()

	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM identity.users") && strings.Contains(sql, "auth_id = $1") {
				return fakeRow{values: []any{
					userID,
					pgtype.Text{String: uid, Valid: true},
					"same@example.com",
					pgtype.Text{String: "Same Name", Valid: true},
					string(domain.StatusActive),
					pgtype.Timestamptz{},
					pgtype.Timestamptz{},
				}}
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{data: [][]any{{string(domain.RoleUser)}}}, nil
			}
			return &fakeRows{}, nil
		},
	}

	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, uid, domain.User{
		AuthID: uid,
		Email:  "same@example.com",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleUser},
	})

	s := New(db, rdb, nil)
	if _, err := s.Sync(ctx, domain.Principal{
		UID:           uid,
		Email:         "same@example.com",
		Name:          "Same Name",
		EmailVerified: true,
	}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, ok := userCache.Get(ctx, uid); ok {
		t.Fatal("cache hit after no-op sync")
	}
}

func TestSyncConflictExhaustionReturnsConflict(t *testing.T) {
	db := &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: pgx.ErrNoRows}
		},
	}

	_, rdb := newTestCache(t)
	s := New(db, rdb, nil)
	_, err := s.Sync(context.Background(), domain.Principal{
		UID:           "uid-sync-conflict",
		Email:         "conflict@example.com",
		EmailVerified: true,
	})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != apperr.KindConflict {
		t.Fatalf("kind = %v, want conflict", appErr.Kind)
	}
}
