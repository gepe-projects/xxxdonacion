package service

import (
	"context"

	"firebase.google.com/go/v4/auth"
)

// fakeFirebase adalah implementasi firebaseAdmin untuk test. Setiap pemanggilan
// dicatat supaya test bisa memastikan cleanup Firebase benar-benar berjalan.
type fakeFirebase struct {
	createUserFn func(context.Context, *auth.UserToCreate) (*auth.UserRecord, error)
	updateUserFn func(context.Context, string, *auth.UserToUpdate) (*auth.UserRecord, error)
	deleteUserFn func(context.Context, string) error

	createErr error
	updateErr error
	deleteErr error

	created []*auth.UserToCreate
	updated []fakeFirebaseUpdate
	deleted []string
}

type fakeFirebaseUpdate struct {
	uid  string
	user *auth.UserToUpdate
}

func (f *fakeFirebase) CreateUser(
	ctx context.Context,
	user *auth.UserToCreate,
) (*auth.UserRecord, error) {
	f.created = append(f.created, user)
	if f.createUserFn != nil {
		return f.createUserFn(ctx, user)
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &auth.UserRecord{UserInfo: &auth.UserInfo{UID: "firebase-uid"}}, nil
}

func (f *fakeFirebase) UpdateUser(
	ctx context.Context,
	uid string,
	user *auth.UserToUpdate,
) (*auth.UserRecord, error) {
	f.updated = append(f.updated, fakeFirebaseUpdate{uid: uid, user: user})
	if f.updateUserFn != nil {
		return f.updateUserFn(ctx, uid, user)
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &auth.UserRecord{UserInfo: &auth.UserInfo{UID: uid}}, nil
}

func (f *fakeFirebase) DeleteUser(ctx context.Context, uid string) error {
	f.deleted = append(f.deleted, uid)
	if f.deleteUserFn != nil {
		return f.deleteUserFn(ctx, uid)
	}
	return f.deleteErr
}
