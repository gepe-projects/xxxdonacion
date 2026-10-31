package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func newCreateUserService(t *testing.T, db *fakeDB, fb firebaseAdmin) *Service {
	t.Helper()
	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	userCache.Set(ctx, "admin-uid", domain.User{
		AuthID: "admin-uid",
		Email:  "admin@example.com",
		Status: domain.StatusActive,
		Roles:  []domain.Role{domain.RoleAdmin},
	})
	return New(db, rdb, fb)
}

func createUserAdmin() domain.Principal {
	return domain.Principal{
		UID:           "admin-uid",
		Email:         "admin@example.com",
		EmailVerified: true,
	}
}

func createUserInput() domain.CreateUserInput {
	return domain.CreateUserInput{
		Email:    "new@example.com",
		Name:     "New User",
		Password: "strongpassword",
	}
}

func userRowValues(id uuid.UUID, authID, email, name, status string) []any {
	now := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
	return []any{
		id,
		pgtype.Text{String: authID, Valid: authID != ""},
		email,
		pgtype.Text{String: name, Valid: name != ""},
		status,
		now,
		now,
	}
}

func insertedUserRow(args ...any) fakeRow {
	id, _ := args[0].(uuid.UUID)
	authID, _ := args[1].(pgtype.Text)
	email, _ := args[2].(string)
	name, _ := args[3].(pgtype.Text)
	status, _ := args[4].(string)
	return fakeRow{values: userRowValues(id, authID.String, email, name.String, status)}
}

func assertErrorKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != want {
		t.Fatalf("kind = %v, want %v", appErr.Kind, want)
	}
}

func TestCreateUserRejectsExistingEmailBeforeFirebase(t *testing.T) {
	existingID := uuid.NewV7()
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "WHERE email = $1") {
				return fakeRow{values: userRowValues(
					existingID, "existing-uid", "existing@example.com", "Existing", string(domain.StatusActive),
				)}
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	}

	// firebaseAdmin nil: test akan panic kalau Firebase dipanggil sebelum
	// pre-check email selesai.
	s := newCreateUserService(t, db, nil)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), domain.CreateUserInput{
		Email:    "existing@example.com",
		Name:     "Existing",
		Password: "strongpassword",
	})

	assertErrorKind(t, err, apperr.KindConflict)
	if db.tx != nil {
		t.Fatal("transaction started before email pre-check")
	}
}

func TestCreateUserPreCheckDatabaseError(t *testing.T) {
	dbErr := errors.New("database unavailable")
	db := &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: dbErr}
		},
	}

	s := newCreateUserService(t, db, &fakeFirebase{})
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindInternal)
	if !errors.Is(err, dbErr) {
		t.Fatalf("error = %v, want wrapped %v", err, dbErr)
	}
	if db.tx != nil {
		t.Fatal("transaction started after pre-check failure")
	}
}

func TestCreateUserSuccess(t *testing.T) {
	fb := &fakeFirebase{}
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WHERE email = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO identity.users"):
				return insertedUserRow(args...)
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
	}

	s := newCreateUserService(t, db, fb)
	user, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if user.AuthID != "firebase-uid" {
		t.Fatalf("auth id = %q, want firebase-uid", user.AuthID)
	}
	if user.Email != "new@example.com" || user.Name != "New User" {
		t.Fatalf("user = %+v, want normalized input", user)
	}
	if user.Status != domain.StatusActive {
		t.Fatalf("status = %v, want active", user.Status)
	}
	if len(user.Roles) != 1 || user.Roles[0] != domain.RoleUser {
		t.Fatalf("roles = %v, want [USER]", user.Roles)
	}
	if db.tx == nil || !db.tx.committed {
		t.Fatalf("transaction state = %+v, want committed", db.tx)
	}
	if len(fb.created) != 1 {
		t.Fatalf("firebase create calls = %d, want 1", len(fb.created))
	}
	if len(fb.deleted) != 0 {
		t.Fatalf("firebase delete calls = %v, want none", fb.deleted)
	}
}

func TestCreateUserFirebaseCreateError(t *testing.T) {
	firebaseErr := errors.New("firebase unavailable")
	fb := &fakeFirebase{createErr: firebaseErr}
	db := &fakeDB{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			return fakeRow{err: pgx.ErrNoRows}
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindInternal)
	if !errors.Is(err, firebaseErr) {
		t.Fatalf("error = %v, want wrapped %v", err, firebaseErr)
	}
	if db.tx != nil {
		t.Fatal("transaction started after firebase create failure")
	}
}

func TestCreateUserInsertConflictCleansUpFirebase(t *testing.T) {
	tests := []struct {
		name      string
		insertErr error
	}{
		{name: "no rows returned", insertErr: pgx.ErrNoRows},
		{name: "unique violation", insertErr: &pgconn.PgError{Code: "23505"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &fakeFirebase{}
			db := &fakeDB{
				queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "WHERE email = $1"):
						return fakeRow{err: pgx.ErrNoRows}
					case strings.Contains(sql, "INSERT INTO identity.users"):
						return fakeRow{err: tt.insertErr}
					case strings.Contains(sql, "WHERE auth_id = $1"):
						return fakeRow{err: pgx.ErrNoRows}
					default:
						return fakeRow{err: pgx.ErrNoRows}
					}
				},
			}

			s := newCreateUserService(t, db, fb)
			_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

			assertErrorKind(t, err, apperr.KindConflict)
			if db.tx == nil || !db.tx.rolledBack {
				t.Fatal("transaction was not rolled back")
			}
			if db.tx.committed {
				t.Fatal("transaction was committed")
			}
			if len(fb.deleted) != 1 || fb.deleted[0] != "firebase-uid" {
				t.Fatalf("firebase deletes = %v, want [firebase-uid]", fb.deleted)
			}
		})
	}
}

func TestCreateUserCleanupGuardSkipsLinkedFirebaseAccount(t *testing.T) {
	fb := &fakeFirebase{}
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WHERE email = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO identity.users"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "WHERE auth_id = $1"):
				// Baris sudah di-link alur lain (mis. /sync).
				return fakeRow{values: userRowValues(
					uuid.NewV7(), "firebase-uid", "new@example.com", "New User", string(domain.StatusActive),
				)}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindConflict)
	if len(fb.deleted) != 0 {
		t.Fatalf("firebase deletes = %v, want none while linked", fb.deleted)
	}
}

func TestCreateUserGrantRoleFailureCleansUpFirebase(t *testing.T) {
	grantErr := errors.New("grant failed")
	fb := &fakeFirebase{}
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WHERE email = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO identity.users"):
				return insertedUserRow(args...)
			case strings.Contains(sql, "WHERE auth_id = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "INSERT INTO identity.user_roles") {
				return pgconn.CommandTag{}, grantErr
			}
			return pgconn.CommandTag{}, nil
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindInternal)
	if !errors.Is(err, grantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, grantErr)
	}
	if db.tx == nil || !db.tx.rolledBack {
		t.Fatal("transaction was not rolled back")
	}
	if len(fb.deleted) != 1 || fb.deleted[0] != "firebase-uid" {
		t.Fatalf("firebase deletes = %v, want [firebase-uid]", fb.deleted)
	}
}

func TestCreateUserCommitFailureCleansUpFirebase(t *testing.T) {
	commitErr := errors.New("commit failed")
	fb := &fakeFirebase{}
	db := &fakeDB{
		commitErr: commitErr,
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WHERE email = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO identity.users"):
				return insertedUserRow(args...)
			case strings.Contains(sql, "WHERE auth_id = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindInternal)
	if !errors.Is(err, commitErr) {
		t.Fatalf("error = %v, want wrapped %v", err, commitErr)
	}
	if db.tx == nil || db.tx.committed {
		t.Fatal("transaction should not be marked committed")
	}
	if len(fb.deleted) != 1 || fb.deleted[0] != "firebase-uid" {
		t.Fatalf("firebase deletes = %v, want [firebase-uid]", fb.deleted)
	}
}

func TestCreateUserCleanupFailureJoinsErrors(t *testing.T) {
	deleteErr := errors.New("delete firebase user failed")
	fb := &fakeFirebase{deleteErr: deleteErr}
	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WHERE email = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "INSERT INTO identity.users"):
				return fakeRow{err: pgx.ErrNoRows}
			case strings.Contains(sql, "WHERE auth_id = $1"):
				return fakeRow{err: pgx.ErrNoRows}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindConflict)
	if !errors.Is(err, deleteErr) {
		t.Fatalf("error = %v, want joined cleanup error %v", err, deleteErr)
	}
	if len(fb.deleted) != 1 {
		t.Fatalf("firebase delete calls = %d, want 1", len(fb.deleted))
	}
}

func TestCreateUserBeginFailureCleansUpFirebase(t *testing.T) {
	beginErr := errors.New("begin failed")
	fb := &fakeFirebase{}
	db := &fakeDB{
		beginErr: beginErr,
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "WHERE auth_id = $1") {
				return fakeRow{err: pgx.ErrNoRows}
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
	}

	s := newCreateUserService(t, db, fb)
	_, err := s.CreateUser(context.Background(), createUserAdmin(), createUserInput())

	assertErrorKind(t, err, apperr.KindInternal)
	if !errors.Is(err, beginErr) {
		t.Fatalf("error = %v, want wrapped %v", err, beginErr)
	}
	if len(fb.deleted) != 1 || fb.deleted[0] != "firebase-uid" {
		t.Fatalf("firebase deletes = %v, want [firebase-uid]", fb.deleted)
	}
	if db.tx != nil {
		t.Fatal("transaction should not be set after begin failure")
	}
}
