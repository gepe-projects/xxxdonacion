package domain

import (
	"errors"
	"slices"
	"testing"

	"xxxdonacion/platform/apperr"
)

func TestUpdateRolesInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		roles   []Role
		want    []Role
		wantErr bool
	}{
		{name: "deduplicates and sorts", roles: []Role{"user", "ADMIN", "admin"}, want: []Role{RoleAdmin, RoleUser}},
		{name: "rejects super admin", roles: []Role{RoleSuperAdmin}, wantErr: true},
		{name: "rejects unknown role", roles: []Role{"OWNER"}, wantErr: true},
		{name: "rejects empty", roles: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := UpdateRolesInput{Roles: tt.roles}
			err := in.Validate()
			if tt.wantErr {
				var appErr *apperr.Error
				if !errors.As(err, &appErr) || appErr.Kind != apperr.KindBadRequest {
					t.Fatalf("error = %v, want bad request", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(in.Roles, tt.want) {
				t.Fatalf("roles = %v, want %v", in.Roles, tt.want)
			}
		})
	}
}
