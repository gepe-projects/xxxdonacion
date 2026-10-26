package service

import (
	"context"
	"errors"
	"testing"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func TestSyncRejectsUnverifiedEmail(t *testing.T) {
	s := New(nil, nil, nil)

	_, err := s.Sync(context.Background(), domain.Principal{
		UID:           "uid-1",
		Email:         "user@example.com",
		EmailVerified: false,
	})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != apperr.KindForbidden {
		t.Fatalf("kind = %v, want forbidden", appErr.Kind)
	}
}

func TestSyncRequiresUID(t *testing.T) {
	s := New(nil, nil, nil)

	_, err := s.Sync(context.Background(), domain.Principal{EmailVerified: true})

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want *apperr.Error", err)
	}
	if appErr.Kind != apperr.KindUnauthorized {
		t.Fatalf("kind = %v, want unauthorized", appErr.Kind)
	}
}
