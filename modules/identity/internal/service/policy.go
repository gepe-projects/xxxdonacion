package service

import (
	"slices"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func hasRole(roles []domain.Role, role domain.Role) bool {
	return slices.Contains(roles, role)
}

// validateTargetModifiable menjaga agar SUPER_ADMIN tidak bisa diubah lewat API
// dan ADMIN hanya bisa mengelola USER.
func validateTargetModifiable(callerRoles, targetRoles []domain.Role) error {
	if hasRole(targetRoles, domain.RoleSuperAdmin) {
		return apperr.Forbidden("SUPER_ADMIN cannot be modified via API")
	}
	if hasRole(callerRoles, domain.RoleSuperAdmin) {
		return nil
	}
	if hasRole(targetRoles, domain.RoleAdmin) {
		return apperr.Forbidden("only SUPER_ADMIN can manage ADMIN")
	}
	return nil
}

func validateRolesGrantable(callerRoles, desiredRoles []domain.Role) error {
	if hasRole(callerRoles, domain.RoleSuperAdmin) {
		return nil
	}
	if hasRole(desiredRoles, domain.RoleAdmin) {
		return apperr.Forbidden("only SUPER_ADMIN can grant ADMIN")
	}
	return nil
}
