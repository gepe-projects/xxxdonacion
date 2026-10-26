package service

import (
	"testing"

	"xxxdonacion/modules/identity/api/domain"
)

func TestValidateTargetModifiable(t *testing.T) {
	tests := []struct {
		name          string
		callerRoles   []domain.Role
		targetRoles   []domain.Role
		wantForbidden bool
	}{
		{name: "admin can manage user", callerRoles: []domain.Role{domain.RoleAdmin}, targetRoles: []domain.Role{domain.RoleUser}},
		{name: "admin cannot manage admin", callerRoles: []domain.Role{domain.RoleAdmin}, targetRoles: []domain.Role{domain.RoleAdmin}, wantForbidden: true},
		{name: "super can manage admin", callerRoles: []domain.Role{domain.RoleSuperAdmin}, targetRoles: []domain.Role{domain.RoleAdmin}},
		{name: "super target is immutable", callerRoles: []domain.Role{domain.RoleSuperAdmin}, targetRoles: []domain.Role{domain.RoleSuperAdmin}, wantForbidden: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTargetModifiable(tt.callerRoles, tt.targetRoles)
			if tt.wantForbidden && err == nil {
				t.Fatal("expected forbidden error")
			}
			if !tt.wantForbidden && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHasAnyRole(t *testing.T) {
	tests := []struct {
		name    string
		user    []domain.Role
		allowed []domain.Role
		want    bool
	}{
		{name: "matches one", user: []domain.Role{domain.RoleUser, domain.RoleAdmin}, allowed: []domain.Role{domain.RoleAdmin}, want: true},
		{name: "no match", user: []domain.Role{domain.RoleUser}, allowed: []domain.Role{domain.RoleAdmin}, want: false},
		{name: "empty allowed", user: []domain.Role{domain.RoleUser}, allowed: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAnyRole(tt.user, tt.allowed); got != tt.want {
				t.Fatalf("hasAnyRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRolesGrantable(t *testing.T) {
	if err := validateRolesGrantable(
		[]domain.Role{domain.RoleAdmin},
		[]domain.Role{domain.RoleAdmin},
	); err == nil {
		t.Fatal("ADMIN should not grant ADMIN")
	}
	if err := validateRolesGrantable(
		[]domain.Role{domain.RoleSuperAdmin},
		[]domain.Role{domain.RoleAdmin},
	); err != nil {
		t.Fatalf("SUPER_ADMIN should grant ADMIN: %v", err)
	}
}
