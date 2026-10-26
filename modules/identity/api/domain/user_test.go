package domain

import (
	"errors"
	"testing"

	"xxxdonacion/platform/apperr"
)

func TestPrincipalValidate(t *testing.T) {
	if err := (Principal{Email: "user@example.com", EmailVerified: true}).Validate(); err == nil {
		t.Fatal("expected missing UID error")
	}
	if err := (Principal{UID: "uid-1", Email: "user@example.com"}).Validate(); err == nil {
		t.Fatal("expected unverified email error")
	}
	if err := (Principal{UID: "uid-1", Email: "user@example.com", EmailVerified: true}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateUserInputValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      CreateUserInput
		want    CreateUserInput
		wantErr bool
	}{
		{
			name: "normalizes input",
			in: CreateUserInput{
				Email:    " User@Example.com ",
				Name:     " User ",
				Password: "strongpassword",
			},
			want: CreateUserInput{
				Email:    "user@example.com",
				Name:     "User",
				Password: "strongpassword",
			},
		},
		{
			name: "password is optional",
			in:   CreateUserInput{Email: "user@example.com", Name: "User"},
			want: CreateUserInput{Email: "user@example.com", Name: "User"},
		},
		{name: "invalid email", in: CreateUserInput{Email: "not-an-email", Password: "strongpassword"}, wantErr: true},
		{name: "short password", in: CreateUserInput{Email: "user@example.com", Password: "123"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
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
			if in.Email != tt.want.Email || in.Name != tt.want.Name || in.Password != tt.want.Password {
				t.Fatalf("input = %+v, want %+v", in, tt.want)
			}
		})
	}
}

func TestUpdateStatusInputValidate(t *testing.T) {
	in := UpdateStatusInput{Status: " disabled "}
	if err := in.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Status != StatusDisabled {
		t.Fatalf("status = %q, want %q", in.Status, StatusDisabled)
	}

	invalid := UpdateStatusInput{Status: "DELETED"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid status error")
	}
}
