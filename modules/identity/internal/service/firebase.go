package service

import (
	"context"

	firebaseauth "firebase.google.com/go/v4/auth"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
)

func (s *Service) createFirebaseUser(ctx context.Context, in domain.CreateUserInput) (string, error) {
	params := (&firebaseauth.UserToCreate{}).
		Email(in.Email).
		EmailVerified(true)
	if in.Name != "" {
		params.DisplayName(in.Name)
	}
	if in.Password != "" {
		params.Password(in.Password)
	}

	user, err := s.auth.CreateUser(ctx, params)
	if err != nil {
		switch {
		case firebaseauth.IsEmailAlreadyExists(err):
			return "", apperr.Conflict("email already exists")
		case firebaseauth.IsUIDAlreadyExists(err):
			return "", apperr.Conflict("firebase account already exists")
		// case firebaseauth.IsInvalidEmail(err): deprecated
		// 	return "", apperr.BadRequest("invalid email")
		default:
			return "", apperr.Internal(err, "create firebase user")
		}
	}
	return user.UID, nil
}

func (s *Service) setFirebaseDisabled(ctx context.Context, uid string, disabled bool) error {
	_, err := s.auth.UpdateUser(ctx, uid, (&firebaseauth.UserToUpdate{}).Disabled(disabled))
	if err != nil {
		if firebaseauth.IsUserNotFound(err) {
			return apperr.Conflict("firebase account not found")
		}
		return apperr.Internal(err, "update firebase user status")
	}
	return nil
}

// deleteFirebaseUser idempoten: user yang sudah tidak ada dianggap sukses,
// supaya retry setelah kegagalan database tidak ikut gagal.
func (s *Service) deleteFirebaseUser(ctx context.Context, uid string) error {
	if err := s.auth.DeleteUser(ctx, uid); err != nil {
		if firebaseauth.IsUserNotFound(err) {
			return nil
		}
		return apperr.Internal(err, "delete firebase user")
	}
	return nil
}
