package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func TestCreateUserRejectsExistingEmailBeforeFirebase(t *testing.T) {
	db := &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: pgx.ErrNoRows}
		},
	}

	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, "admin-uid", domain.User{
		AuthID: "admin-uid",
		Email:  "admin@example.com",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleAdmin},
	})

	// auth nil: test gagal kalau Firebase dipanggil sebelum cek database.
	s := New(db, rdb, nil)
	_, err := s.CreateUser(ctx, domain.Principal{
		UID:           "admin-uid",
		Email:         "admin@example.com",
		EmailVerified: true,
	}, domain.CreateUserInput{
		Email:    "existing@example.com",
		Name:     "Existing",
		Password: "strongpassword",
	})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
	if db.tx == nil || !db.tx.rolledBack {
		t.Fatal("transaction was not rolled back")
	}
}
