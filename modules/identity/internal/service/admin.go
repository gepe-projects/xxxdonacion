package service

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/repository"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/pagination"
	"xxxdonacion/platform/postgres"
)

func (s *Service) CreateUser(
	ctx context.Context,
	caller domain.Principal,
	in domain.CreateUserInput,
) (*domain.User, error) {
	if _, err := s.authorizeAdmin(ctx, caller); err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	// Email dicek di luar transaksi agar email yang sudah terdaftar ditolak
	// sebelum Firebase membuat akun yang nanti harus dibersihkan.
	if _, err := s.repo.GetUserByEmail(ctx, in.Email); err == nil {
		return nil, apperr.Conflict("email already exists")
	} else if !postgres.IsNoRows(err) {
		return nil, apperr.Internal(err, "get user by email")
	}

	// Akun Firebase dibuat lebih dulu agar auth_id berasal dari Firebase,
	// bukan dibuat sendiri oleh aplikasi.
	firebaseUID, err := s.createFirebaseUser(ctx, in)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, s.joinFirebaseCleanup(
			ctx,
			firebaseUID,
			apperr.Internal(err, "begin create user transaction"),
		)
	}
	// Rollback setelah commit sukses atau transaksi yang sudah tertutup
	// hanyalah no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.repo.WithTx(tx)
	user, err := q.CreateUserIfAbsent(ctx, repository.CreateUserIfAbsentParams{
		ID:     uuid.NewV7(),
		AuthID: postgres.ToPgText(firebaseUID),
		Email:  in.Email,
		Name:   postgres.ToPgTextPtr(&in.Name),
		Status: string(domain.StatusActive),
	})
	if err != nil {
		switch {
		case postgres.IsNoRows(err) || postgres.IsUniqueViolation(err):
			return nil, s.abortCreateUser(
				ctx,
				tx,
				firebaseUID,
				apperr.Conflict("email or firebase account already exists"),
			)
		default:
			return nil, s.abortCreateUser(ctx, tx, firebaseUID, apperr.Internal(err, "create user"))
		}
	}

	if err := q.GrantRole(ctx, repository.GrantRoleParams{
		UserID: user.ID,
		Role:   string(domain.RoleUser),
	}); err != nil {
		return nil, s.abortCreateUser(ctx, tx, firebaseUID, apperr.Internal(err, "grant user role"))
	}

	if err := tx.Commit(ctx); err != nil {
		// Commit yang gagal sudah menutup transaksi; rollback yang ditunda
		// hanya no-op. Akun Firebase tidak boleh hidup tanpa baris database.
		return nil, s.joinFirebaseCleanup(
			ctx,
			firebaseUID,
			apperr.Internal(err, "commit create user transaction"),
		)
	}

	return toUser(user, []domain.Role{domain.RoleUser}), nil
}

// abortCreateUser melakukan rollback transaksi secara eksplisit sebelum
// cleanup. Rollback harus selesai lebih dulu agar baris yang seharusnya
// dibatalkan tidak dianggap milik alur lain oleh guard cleanupFirebaseUser.
func (s *Service) abortCreateUser(
	ctx context.Context,
	tx pgx.Tx,
	firebaseUID string,
	primary error,
) error {
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		primary = errors.Join(primary, fmt.Errorf("rollback create user transaction: %w", err))
	}
	return s.joinFirebaseCleanup(ctx, firebaseUID, primary)
}

// joinFirebaseCleanup menjalankan cleanup Firebase dan menggabungkan
// kegagalannya dengan error utama.
func (s *Service) joinFirebaseCleanup(ctx context.Context, uid string, primary error) error {
	if err := s.cleanupFirebaseUser(ctx, uid); err != nil {
		return errors.Join(primary, err)
	}
	return primary
}

// cleanupFirebaseUser menghapus akun Firebase hanya kalau auth_id tersebut
// belum dipakai baris database. Baris yang sudah ada bisa berasal dari alur
// lain seperti /sync yang sudah me-link akun itu, sehingga akun Firebase harus
// dibiarkan hidup.
func (s *Service) cleanupFirebaseUser(ctx context.Context, uid string) error {
	_, err := s.repo.GetUserByAuthID(ctx, postgres.ToPgText(uid))
	switch {
	case err == nil:
		return nil
	case postgres.IsNoRows(err):
		return s.deleteFirebaseUser(ctx, uid)
	default:
		return apperr.Internal(err, "check firebase user linkage")
	}
}

func (s *Service) SetUserStatus(
	ctx context.Context,
	caller domain.Principal,
	userID uuid.UUID,
	in domain.UpdateStatusInput,
) (*domain.User, error) {
	callerUser, err := s.authorizeAdmin(ctx, caller)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	target, err := s.loadUserWithRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	if target.ID == callerUser.ID {
		return nil, apperr.Forbidden("cannot change your own status")
	}
	if err := validateTargetModifiable(callerUser.Roles, target.Roles); err != nil {
		return nil, err
	}
	if target.AuthID == "" {
		return nil, apperr.Conflict("user has no linked firebase account")
	}
	if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
		return nil, apperr.Internal(err, "invalidate user cache")
	}

	if err := s.setFirebaseDisabled(ctx, target.AuthID, in.Status == domain.StatusDisabled); err != nil {
		return nil, err
	}

	updated, err := s.repo.UpdateUserStatus(ctx, repository.UpdateUserStatusParams{
		ID:     target.ID,
		Status: string(in.Status),
	})
	if err != nil {
		// Best effort: jangan biarkan Firebase dan database berbeda status.
		if revertErr := s.setFirebaseDisabled(ctx, target.AuthID, target.Status == domain.StatusDisabled); revertErr != nil {
			return nil, errors.Join(apperr.Internal(err, "update user status"), revertErr)
		}
		return nil, apperr.Internal(err, "update user status")
	}

	if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
		return nil, apperr.Internal(err, "invalidate user cache")
	}
	target.Status = in.Status
	target.UpdatedAt = updated.UpdatedAt.Time
	return &target, nil
}

func (s *Service) SetUserRoles(
	ctx context.Context,
	caller domain.Principal,
	userID uuid.UUID,
	in domain.UpdateRolesInput,
) (*domain.User, error) {
	callerUser, err := s.authorizeAdmin(ctx, caller)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	target, err := s.loadUserWithRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	if target.ID == callerUser.ID {
		return nil, apperr.Forbidden("cannot change your own roles")
	}
	if err := validateTargetModifiable(callerUser.Roles, target.Roles); err != nil {
		return nil, err
	}
	if err := validateRolesGrantable(callerUser.Roles, in.Roles); err != nil {
		return nil, err
	}
	if target.AuthID != "" {
		if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
			return nil, apperr.Internal(err, "invalidate user cache")
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperr.Internal(err, "begin role transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.repo.WithTx(tx)
	if err := q.DeleteUserRoles(ctx, target.ID); err != nil {
		return nil, apperr.Internal(err, "delete user roles")
	}
	for _, role := range in.Roles {
		if err := q.GrantRole(ctx, repository.GrantRoleParams{
			UserID: target.ID,
			Role:   string(role),
		}); err != nil {
			return nil, apperr.Internal(err, "grant user role")
		}
	}
	applied, err := q.ListRolesByUserID(ctx, target.ID)
	if err != nil {
		return nil, apperr.Internal(err, "list user roles")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.Internal(err, "commit role transaction")
	}

	if target.AuthID != "" {
		if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
			return nil, apperr.Internal(err, "invalidate user cache")
		}
	}
	target.Roles = toRoles(applied)
	return &target, nil
}

func (s *Service) DeleteUser(
	ctx context.Context,
	caller domain.Principal,
	userID uuid.UUID,
) error {
	callerUser, err := s.authorizeAdmin(ctx, caller)
	if err != nil {
		return err
	}

	target, err := s.loadUserWithRoles(ctx, userID)
	if err != nil {
		return err
	}
	if target.ID == callerUser.ID {
		return apperr.Forbidden("cannot delete yourself")
	}
	if err := validateTargetModifiable(callerUser.Roles, target.Roles); err != nil {
		return err
	}

	if target.AuthID != "" {
		// Cache dihapus sebelum penghapusan agar kegagalan invalidasi tidak
		// meninggalkan user yang sudah dihapus tetap memiliki akses.
		if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
			return apperr.Internal(err, "invalidate user cache")
		}
		if err := s.deleteFirebaseUser(ctx, target.AuthID); err != nil {
			return err
		}
	}
	if err := s.repo.DeleteUser(ctx, target.ID); err != nil {
		return apperr.Internal(err, "delete user")
	}
	if target.AuthID != "" {
		// Invalidasi ulang untuk menutup request yang sempat mengisi cache
		// lama di antara invalidasi pertama dan penghapusan database.
		if err := s.cache.Invalidate(ctx, target.AuthID); err != nil {
			return apperr.Internal(err, "invalidate user cache")
		}
	}
	return nil
}

func (s *Service) ListUsers(
	ctx context.Context,
	caller domain.Principal,
	limit int,
	cursor *uuid.UUID,
) (pagination.Page[domain.User], error) {
	if _, err := s.authorizeAdmin(ctx, caller); err != nil {
		return pagination.Page[domain.User]{}, err
	}

	if limit <= 0 {
		limit = pagination.DefaultLimit
	}
	if limit > pagination.MaxLimit {
		limit = pagination.MaxLimit
	}
	fetch := int32(limit + 1)

	users := make([]domain.User, 0, limit+1)
	if cursor != nil {
		rows, err := s.repo.ListUsersAfter(ctx, repository.ListUsersAfterParams{
			ID:    *cursor,
			Limit: fetch,
		})
		if err != nil {
			return pagination.Page[domain.User]{}, apperr.Internal(err, "list users")
		}
		for _, row := range rows {
			users = append(users, userFromListAfterRow(row))
		}
	} else {
		rows, err := s.repo.ListUsers(ctx, fetch)
		if err != nil {
			return pagination.Page[domain.User]{}, apperr.Internal(err, "list users")
		}
		for _, row := range rows {
			users = append(users, userFromListRow(row))
		}
	}

	return pagination.NewPage(users, limit, func(user domain.User) string {
		return pagination.EncodeUUID(user.ID)
	}), nil
}

// userFromListRow memetakan baris ListUsers menjadi domain.User.
func userFromListRow(row repository.ListUsersRow) domain.User {
	return *toUser(repository.IdentityUser{
		ID:        row.ID,
		AuthID:    row.AuthID,
		Email:     row.Email,
		Name:      row.Name,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, toRoles(row.Roles))
}

// userFromListAfterRow memetakan baris ListUsersAfter menjadi domain.User.
func userFromListAfterRow(row repository.ListUsersAfterRow) domain.User {
	return *toUser(repository.IdentityUser{
		ID:        row.ID,
		AuthID:    row.AuthID,
		Email:     row.Email,
		Name:      row.Name,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, toRoles(row.Roles))
}

func (s *Service) authorizeAdmin(
	ctx context.Context,
	caller domain.Principal,
) (domain.User, error) {
	// Email verified tetap dicek untuk endpoint admin; Authorize sendiri
	// hanya mengecek status dan role.
	if err := caller.Validate(); err != nil {
		return domain.User{}, err
	}

	user, err := s.Authorize(ctx, caller.UID, domain.RoleAdmin, domain.RoleSuperAdmin)
	if err != nil {
		return domain.User{}, err
	}
	return *user, nil
}

func (s *Service) loadUserWithRoles(
	ctx context.Context,
	userID uuid.UUID,
) (domain.User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if postgres.IsNoRows(err) {
			return domain.User{}, apperr.NotFound("user not found")
		}
		return domain.User{}, apperr.Internal(err, "get user by id")
	}
	roles, err := s.repo.ListRolesByUserID(ctx, user.ID)
	if err != nil {
		return domain.User{}, apperr.Internal(err, "list target roles")
	}
	return *toUser(user, toRoles(roles)), nil
}
