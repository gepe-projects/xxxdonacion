package service

import (
	"context"
	"slices"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/postgres"
)

// Authorize adalah pintu authz lintas module. Cache Redis dipakai lebih dulu;
// database hanya di-hit saat cache miss.
func (s *Service) Authorize(
	ctx context.Context,
	uid string,
	roles ...domain.Role,
) (*domain.User, error) {
	if uid == "" {
		return nil, apperr.Unauthorized("unauthenticated")
	}

	user, ok := s.cache.Get(ctx, uid)
	if !ok {
		loaded, err := s.loadUserByAuthID(ctx, uid)
		if err != nil {
			return nil, err
		}
		user = *loaded
		s.cache.Set(ctx, uid, user)
	}

	if user.Status != domain.StatusActive {
		return nil, apperr.Forbidden("account is disabled")
	}
	if len(roles) > 0 && !hasAnyRole(user.Roles, roles) {
		return nil, apperr.Forbidden("insufficient role")
	}
	return &user, nil
}

func (s *Service) loadUserByAuthID(ctx context.Context, uid string) (*domain.User, error) {
	user, err := s.repo.GetUserByAuthID(ctx, postgres.ToPgText(uid))
	if err != nil {
		if postgres.IsNoRows(err) {
			return nil, apperr.Forbidden("user is not provisioned; call sync first")
		}
		return nil, apperr.Internal(err, "get user by auth id")
	}
	roles, err := s.repo.ListRolesByUserID(ctx, user.ID)
	if err != nil {
		return nil, apperr.Internal(err, "list user roles")
	}
	return toUser(user, toRoles(roles)), nil
}

func hasAnyRole(userRoles, allowed []domain.Role) bool {
	for _, role := range allowed {
		if slices.Contains(userRoles, role) {
			return true
		}
	}
	return false
}
