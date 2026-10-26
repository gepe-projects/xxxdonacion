package service

import (
	"context"
	"errors"
	"strings"
	"uuid"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/repository"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/postgres"
)

const syncConflictRetries = 3

func (s *Service) Sync(ctx context.Context, caller domain.Principal) (*domain.User, error) {
	if err := caller.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, apperr.Internal(err, "begin sync transaction")
	}
	// Rollback is a no-op after a successful commit.
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.repo.WithTx(tx)

	user, err := s.findOrCreateUser(ctx, q, caller)
	if err != nil {
		return nil, err
	}
	if domain.Status(user.Status) != domain.StatusActive {
		return nil, apperr.Forbidden("account is disabled")
	}

	user, err = s.applyProfile(ctx, q, user, caller)
	if err != nil {
		return nil, err
	}

	storedRoles, err := q.ListRolesByUserID(ctx, user.ID)
	if err != nil {
		return nil, apperr.Internal(err, "list user roles")
	}
	roles := toRoles(storedRoles)
	if len(roles) == 0 {
		if err := q.GrantRole(ctx, repository.GrantRoleParams{
			UserID: user.ID,
			Role:   string(domain.RoleUser),
		}); err != nil {
			return nil, apperr.Internal(err, "grant default role")
		}
		roles = []domain.Role{domain.RoleUser}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, apperr.Internal(err, "commit sync transaction")
	}

	return toUser(user, roles), nil
}

func (s *Service) findOrCreateUser(
	ctx context.Context,
	q *repository.Queries,
	caller domain.Principal,
) (repository.IdentityUser, error) {
	user, err := q.GetUserByAuthID(ctx, postgres.ToPgText(caller.UID))
	if err == nil {
		return user, nil
	}
	if !postgres.IsNoRows(err) {
		return repository.IdentityUser{}, apperr.Internal(err, "get user by auth id")
	}

	user, err = q.GetUserByEmail(ctx, caller.Email)
	if err == nil {
		return s.linkOrConflict(ctx, q, user, caller)
	}
	if !postgres.IsNoRows(err) {
		return repository.IdentityUser{}, apperr.Internal(err, "get user by email")
	}

	created, err := q.CreateUserIfAbsent(ctx, repository.CreateUserIfAbsentParams{
		ID:     uuid.NewV7(),
		AuthID: postgres.ToPgText(caller.UID),
		Email:  caller.Email,
		Name:   postgres.ToPgTextPtr(&caller.Name),
		Status: string(domain.StatusActive),
	})
	if err == nil {
		if err := q.GrantRole(ctx, repository.GrantRoleParams{
			UserID: created.ID,
			Role:   string(domain.RoleUser),
		}); err != nil {
			return repository.IdentityUser{}, apperr.Internal(err, "grant default role")
		}
		return created, nil
	}
	if !postgres.IsNoRows(err) {
		return repository.IdentityUser{}, apperr.Internal(err, "create user")
	}

	// Another transaction inserted a row with the same auth_id or email.
	return s.resolveSyncConflict(ctx, q, caller)
}

func (s *Service) linkOrConflict(
	ctx context.Context,
	q *repository.Queries,
	user repository.IdentityUser,
	caller domain.Principal,
) (repository.IdentityUser, error) {
	if user.AuthID.Valid {
		if user.AuthID.String == caller.UID {
			return user, nil
		}
		return repository.IdentityUser{}, apperr.Conflict("email is already linked to another account")
	}

	linked, err := q.LinkUserAuthID(ctx, repository.LinkUserAuthIDParams{
		ID:     user.ID,
		AuthID: postgres.ToPgText(caller.UID),
	})
	if err == nil {
		return linked, nil
	}
	if !postgres.IsNoRows(err) {
		return repository.IdentityUser{}, apperr.Internal(err, "link auth id")
	}

	// Lost the race: another request linked the row first.
	return s.resolveSyncConflict(ctx, q, caller)
}

func (s *Service) resolveSyncConflict(
	ctx context.Context,
	q *repository.Queries,
	caller domain.Principal,
) (repository.IdentityUser, error) {
	for range syncConflictRetries {
		user, err := q.GetUserByAuthID(ctx, postgres.ToPgText(caller.UID))
		if err == nil {
			return user, nil
		}
		if !postgres.IsNoRows(err) {
			return repository.IdentityUser{}, apperr.Internal(err, "get user by auth id")
		}

		user, err = q.GetUserByEmail(ctx, caller.Email)
		switch {
		case err == nil:
			if user.AuthID.Valid {
				return repository.IdentityUser{}, apperr.Conflict("email is already linked to another account")
			}
			linked, linkErr := q.LinkUserAuthID(ctx, repository.LinkUserAuthIDParams{
				ID:     user.ID,
				AuthID: postgres.ToPgText(caller.UID),
			})
			switch {
			case linkErr == nil:
				return linked, nil
			case postgres.IsNoRows(linkErr):
				// Another request linked it between select and update.
			default:
				return repository.IdentityUser{}, apperr.Internal(linkErr, "link auth id")
			}
		case postgres.IsNoRows(err):
			// Row still absent; try the next iteration.
		default:
			return repository.IdentityUser{}, apperr.Internal(err, "get user by email")
		}
	}

	return repository.IdentityUser{}, apperr.Internal(errors.New("too many conflicts"), "resolve concurrent sync")
}

func (s *Service) applyProfile(
	ctx context.Context,
	q *repository.Queries,
	user repository.IdentityUser,
	caller domain.Principal,
) (repository.IdentityUser, error) {
	emailChanged := !strings.EqualFold(user.Email, caller.Email)
	nameChanged := caller.Name != "" && user.Name.String != caller.Name
	if !emailChanged && !nameChanged {
		return user, nil
	}

	name := user.Name
	if caller.Name != "" {
		name = postgres.ToPgTextPtr(&caller.Name)
	}

	updated, err := q.UpdateUserProfile(ctx, repository.UpdateUserProfileParams{
		ID:    user.ID,
		Email: caller.Email,
		Name:  name,
	})
	if err != nil {
		if postgres.IsUniqueViolation(err) {
			return repository.IdentityUser{}, apperr.Conflict("email is already in use")
		}
		return repository.IdentityUser{}, apperr.Internal(err, "update user profile")
	}
	return updated, nil
}
