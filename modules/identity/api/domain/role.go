package domain

import (
	"slices"
	"strings"

	"xxxdonacion/platform/apperr"
)

type Role string

const (
	RoleUser       Role = "USER"
	RoleAdmin      Role = "ADMIN"
	RoleSuperAdmin Role = "SUPER_ADMIN"
)

func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAdmin, RoleSuperAdmin:
		return true
	default:
		return false
	}
}

// Assignable false untuk SUPER_ADMIN: role itu hanya dibuat manual dari database.
func (r Role) Assignable() bool {
	return r == RoleUser || r == RoleAdmin
}

type UpdateRolesInput struct {
	Roles []Role
}

// Validate memvalidasi sekaligus menormalkan daftar role.
func (in *UpdateRolesInput) Validate() error {
	roles, err := normalizeRoles(in.Roles)
	if err != nil {
		return err
	}
	in.Roles = roles
	return nil
}

func normalizeRoles(input []Role) ([]Role, error) {
	if len(input) == 0 {
		return nil, apperr.BadRequest("at least one role is required")
	}

	seen := make(map[Role]struct{}, len(input))
	roles := make([]Role, 0, len(input))
	for _, raw := range input {
		role := Role(strings.ToUpper(strings.TrimSpace(string(raw))))
		if !role.Valid() {
			return nil, apperr.BadRequest("invalid role")
		}
		if !role.Assignable() {
			return nil, apperr.BadRequest("SUPER_ADMIN cannot be assigned via API")
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}

	slices.Sort(roles)
	return roles, nil
}
