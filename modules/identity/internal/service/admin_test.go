package service

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/pagination"
)

func adminTestUser(id uuid.UUID, roles ...domain.Role) domain.User {
	return domain.User{
		ID:     id,
		AuthID: "admin-uid",
		Email:  "admin@example.com",
		Status: domain.StatusActive,
		Roles:  roles,
	}
}

func principalFor(u domain.User) domain.Principal {
	return domain.Principal{UID: u.AuthID, Email: u.Email, EmailVerified: true}
}

func newAdminTestService(t *testing.T, db *fakeDB, fb firebaseAdmin, callers ...domain.User) *Service {
	t.Helper()
	userCache, rdb := newTestCache(t)
	ctx := context.Background()
	for _, caller := range callers {
		userCache.Set(ctx, caller.AuthID, caller)
	}
	return New(db, rdb, fb)
}

type adminTarget struct {
	id     uuid.UUID
	authID string
	status domain.Status
	roles  []string
}

func (target adminTarget) row() fakeRow {
	now := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
	return fakeRow{values: []any{
		target.id,
		pgtype.Text{String: target.authID, Valid: target.authID != ""},
		"target@example.com",
		pgtype.Text{String: "Target", Valid: true},
		string(target.status),
		now,
		now,
	}}
}

// adminTargetDB menjawab GetUserByID dan ListRolesByUserID untuk target.
func adminTargetDB(target adminTarget) *fakeDB {
	return &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "SELECT id, auth_id, email") {
				return target.row()
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				rows := make([][]any, 0, len(target.roles))
				for _, role := range target.roles {
					rows = append(rows, []any{role})
				}
				return &fakeRows{data: rows}, nil
			}
			return &fakeRows{}, nil
		},
		execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}
}

func TestSetUserStatusEarlyExits(t *testing.T) {
	callerID := uuid.NewV7()
	otherID := uuid.NewV7()
	disabledCaller := adminTestUser(callerID, domain.RoleAdmin)
	disabledCaller.Status = domain.StatusDisabled

	tests := []struct {
		name     string
		caller   domain.User
		target   adminTarget
		status   domain.Status
		wantKind apperr.Kind
	}{
		{
			name:     "cannot change own status",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: callerID, authID: "admin-uid", status: domain.StatusActive, roles: []string{"USER"}},
			status:   domain.StatusDisabled,
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "super admin cannot be modified",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"SUPER_ADMIN"}},
			status:   domain.StatusDisabled,
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "admin cannot manage admin",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"ADMIN"}},
			status:   domain.StatusDisabled,
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "disabled caller",
			caller:   disabledCaller,
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
			status:   domain.StatusDisabled,
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "invalid status",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
			status:   domain.Status("BOGUS"),
			wantKind: apperr.KindBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := adminTargetDB(tt.target)
			fb := &fakeFirebase{}
			s := newAdminTestService(t, db, fb, tt.caller)

			_, err := s.SetUserStatus(
				context.Background(),
				principalFor(tt.caller),
				tt.target.id,
				domain.UpdateStatusInput{Status: tt.status},
			)

			assertErrorKind(t, err, tt.wantKind)
			if db.tx != nil {
				t.Fatal("unexpected transaction")
			}
			if len(fb.updated) != 0 {
				t.Fatalf("firebase update calls = %d, want none", len(fb.updated))
			}
		})
	}
}

func TestSetUserStatusUpdatesFirebaseAndDatabase(t *testing.T) {
	callerID := uuid.NewV7()
	targetID := uuid.NewV7()
	now := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}

	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT id, auth_id, email"):
				return adminTarget{
					id: targetID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"},
				}.row()
			case strings.Contains(sql, "UPDATE identity.users"):
				return fakeRow{values: []any{
					targetID,
					pgtype.Text{String: "target-uid", Valid: true},
					"target@example.com",
					pgtype.Text{String: "Target", Valid: true},
					string(domain.StatusDisabled),
					now,
					now,
				}}
			default:
				return fakeRow{err: pgx.ErrNoRows}
			}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{data: [][]any{{"USER"}}}, nil
			}
			return &fakeRows{}, nil
		},
	}

	caller := adminTestUser(callerID, domain.RoleAdmin)
	fb := &fakeFirebase{}
	s := newAdminTestService(t, db, fb, caller)

	user, err := s.SetUserStatus(
		context.Background(),
		principalFor(caller),
		targetID,
		domain.UpdateStatusInput{Status: domain.StatusDisabled},
	)
	if err != nil {
		t.Fatalf("set user status: %v", err)
	}
	if user.Status != domain.StatusDisabled {
		t.Fatalf("status = %v, want disabled", user.Status)
	}
	if len(fb.updated) != 1 || fb.updated[0].uid != "target-uid" {
		t.Fatalf("firebase updates = %v, want target-uid", fb.updated)
	}
	if db.tx != nil {
		t.Fatal("status update should not start a transaction")
	}
}

func TestSetUserRolesEarlyExits(t *testing.T) {
	callerID := uuid.NewV7()
	otherID := uuid.NewV7()
	disabledCaller := adminTestUser(callerID, domain.RoleAdmin)
	disabledCaller.Status = domain.StatusDisabled

	tests := []struct {
		name     string
		caller   domain.User
		target   adminTarget
		roles    []domain.Role
		wantKind apperr.Kind
	}{
		{
			name:     "cannot change own roles",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: callerID, authID: "admin-uid", status: domain.StatusActive, roles: []string{"USER"}},
			roles:    []domain.Role{domain.RoleUser},
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "super admin cannot be modified",
			caller:   adminTestUser(callerID, domain.RoleSuperAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"SUPER_ADMIN"}},
			roles:    []domain.Role{domain.RoleUser},
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "admin cannot manage admin",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"ADMIN"}},
			roles:    []domain.Role{domain.RoleUser},
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "admin cannot grant admin",
			caller:   adminTestUser(callerID, domain.RoleAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
			roles:    []domain.Role{domain.RoleAdmin},
			wantKind: apperr.KindForbidden,
		},
		{
			name:     "super admin role cannot be assigned",
			caller:   adminTestUser(callerID, domain.RoleSuperAdmin),
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
			roles:    []domain.Role{domain.RoleSuperAdmin},
			wantKind: apperr.KindBadRequest,
		},
		{
			name:     "disabled caller",
			caller:   disabledCaller,
			target:   adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
			roles:    []domain.Role{domain.RoleUser},
			wantKind: apperr.KindForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := adminTargetDB(tt.target)
			fb := &fakeFirebase{}
			s := newAdminTestService(t, db, fb, tt.caller)

			_, err := s.SetUserRoles(
				context.Background(),
				principalFor(tt.caller),
				tt.target.id,
				domain.UpdateRolesInput{Roles: tt.roles},
			)

			assertErrorKind(t, err, tt.wantKind)
			if db.tx != nil {
				t.Fatal("unexpected transaction")
			}
			if len(fb.updated) != 0 {
				t.Fatalf("firebase update calls = %d, want none", len(fb.updated))
			}
		})
	}
}

func TestSetUserRolesReplacesRoles(t *testing.T) {
	callerID := uuid.NewV7()
	targetID := uuid.NewV7()
	var deleted bool
	var granted bool

	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "SELECT id, auth_id, email") {
				return adminTarget{
					id: targetID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"},
				}.row()
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{data: [][]any{{"ADMIN"}}}, nil
			}
			return &fakeRows{}, nil
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "DELETE FROM identity.user_roles"):
				deleted = true
			case strings.Contains(sql, "INSERT INTO identity.user_roles"):
				granted = true
			}
			return pgconn.CommandTag{}, nil
		},
	}

	caller := adminTestUser(callerID, domain.RoleSuperAdmin)
	fb := &fakeFirebase{}
	s := newAdminTestService(t, db, fb, caller)

	user, err := s.SetUserRoles(
		context.Background(),
		principalFor(caller),
		targetID,
		domain.UpdateRolesInput{Roles: []domain.Role{domain.RoleAdmin}},
	)
	if err != nil {
		t.Fatalf("set user roles: %v", err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != domain.RoleAdmin {
		t.Fatalf("roles = %v, want [ADMIN]", user.Roles)
	}
	if !deleted || !granted {
		t.Fatalf("delete roles = %v, grant role = %v, want both true", deleted, granted)
	}
	if db.tx == nil || !db.tx.committed {
		t.Fatalf("transaction state = %+v, want committed", db.tx)
	}
	if len(fb.updated) != 0 {
		t.Fatalf("firebase update calls = %d, want none", len(fb.updated))
	}
}

func TestDeleteUserEarlyExits(t *testing.T) {
	callerID := uuid.NewV7()
	otherID := uuid.NewV7()
	disabledCaller := adminTestUser(callerID, domain.RoleAdmin)
	disabledCaller.Status = domain.StatusDisabled

	tests := []struct {
		name   string
		caller domain.User
		target adminTarget
	}{
		{
			name:   "cannot delete yourself",
			caller: adminTestUser(callerID, domain.RoleAdmin),
			target: adminTarget{id: callerID, authID: "admin-uid", status: domain.StatusActive, roles: []string{"USER"}},
		},
		{
			name:   "super admin cannot be deleted",
			caller: adminTestUser(callerID, domain.RoleSuperAdmin),
			target: adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"SUPER_ADMIN"}},
		},
		{
			name:   "admin cannot delete admin",
			caller: adminTestUser(callerID, domain.RoleAdmin),
			target: adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"ADMIN"}},
		},
		{
			name:   "disabled caller",
			caller: disabledCaller,
			target: adminTarget{id: otherID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := adminTargetDB(tt.target)
			fb := &fakeFirebase{}
			s := newAdminTestService(t, db, fb, tt.caller)

			err := s.DeleteUser(context.Background(), principalFor(tt.caller), tt.target.id)

			assertErrorKind(t, err, apperr.KindForbidden)
			if len(fb.deleted) != 0 {
				t.Fatalf("firebase deletes = %v, want none", fb.deleted)
			}
		})
	}
}

func TestDeleteUserRemovesFirebaseAndDatabase(t *testing.T) {
	callerID := uuid.NewV7()
	targetID := uuid.NewV7()
	var deleted bool

	db := &fakeDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "SELECT id, auth_id, email") {
				return adminTarget{
					id: targetID, authID: "target-uid", status: domain.StatusActive, roles: []string{"USER"},
				}.row()
			}
			return fakeRow{err: pgx.ErrNoRows}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM identity.user_roles") {
				return &fakeRows{data: [][]any{{"USER"}}}, nil
			}
			return &fakeRows{}, nil
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "DELETE FROM identity.users") {
				deleted = true
			}
			return pgconn.CommandTag{}, nil
		},
	}

	caller := adminTestUser(callerID, domain.RoleAdmin)
	fb := &fakeFirebase{}
	s := newAdminTestService(t, db, fb, caller)

	if err := s.DeleteUser(context.Background(), principalFor(caller), targetID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if !deleted {
		t.Fatal("database delete was not executed")
	}
	if len(fb.deleted) != 1 || fb.deleted[0] != "target-uid" {
		t.Fatalf("firebase deletes = %v, want [target-uid]", fb.deleted)
	}
}

func listUserRow(id uuid.UUID, email string, roles []string) []any {
	now := pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true}
	return []any{
		id,
		pgtype.Text{String: "uid-" + email, Valid: true},
		email,
		pgtype.Text{String: "Name " + email, Valid: true},
		string(domain.StatusActive),
		now,
		now,
		roles,
	}
}

func TestListUsersPaginationAndRoleMapping(t *testing.T) {
	caller := adminTestUser(uuid.NewV7(), domain.RoleAdmin)
	ids := []uuid.UUID{uuid.NewV7(), uuid.NewV7(), uuid.NewV7()}

	db := &fakeDB{
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "WHERE u.id < $1") {
				return &fakeRows{data: [][]any{
					listUserRow(ids[1], "b@example.com", []string{"USER"}),
					listUserRow(ids[2], "c@example.com", []string{}),
				}}, nil
			}
			return &fakeRows{data: [][]any{
				listUserRow(ids[0], "a@example.com", []string{"ADMIN", "USER"}),
				listUserRow(ids[1], "b@example.com", []string{"USER"}),
				listUserRow(ids[2], "c@example.com", []string{}),
			}}, nil
		},
	}

	s := newAdminTestService(t, db, nil, caller)
	page, err := s.ListUsers(context.Background(), principalFor(caller), 2, nil)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(page.Items) != 2 || !page.HasMore {
		t.Fatalf("page = %+v, want 2 items with more", page)
	}
	if page.NextCursor != pagination.EncodeUUID(ids[1]) {
		t.Fatalf("next cursor = %q, want %q", page.NextCursor, pagination.EncodeUUID(ids[1]))
	}
	if want := []domain.Role{domain.RoleAdmin, domain.RoleUser}; !slices.Equal(page.Items[0].Roles, want) {
		t.Fatalf("roles = %v, want %v", page.Items[0].Roles, want)
	}

	cursor := ids[1]
	after, err := s.ListUsers(context.Background(), principalFor(caller), 2, &cursor)
	if err != nil {
		t.Fatalf("list users after cursor: %v", err)
	}
	if len(after.Items) != 2 || after.HasMore || after.NextCursor != "" {
		t.Fatalf("page after cursor = %+v, want final page", after)
	}
	if after.Items[0].Email != "b@example.com" {
		t.Fatalf("first after item = %q, want b@example.com", after.Items[0].Email)
	}
}
