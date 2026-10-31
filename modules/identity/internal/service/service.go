package service

import (
	"context"

	"firebase.google.com/go/v4/auth"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/cache"
	"xxxdonacion/modules/identity/internal/repository"
)

// DB adalah seam kecil supaya service bisa dites dengan pgxmock. Implementasi
// produksinya tetap *pgxpool.Pool.
type DB interface {
	repository.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// firebaseAdmin adalah seam operasi Firebase Admin yang dipakai service.
// *auth.Client memenuhi interface ini tanpa adapter.
type firebaseAdmin interface {
	CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error)
	UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error)
	DeleteUser(ctx context.Context, uid string) error
}

type Service struct {
	db    DB
	repo  *repository.Queries
	cache *cache.UserCache
	auth  firebaseAdmin
}

func New(db DB, rdb *redis.Client, authClient firebaseAdmin) *Service {
	return &Service{
		db:    db,
		repo:  repository.New(db),
		cache: cache.NewUserCache(rdb),
		auth:  authClient,
	}
}

func toRoles(roles []string) []domain.Role {
	out := make([]domain.Role, 0, len(roles))
	for _, role := range roles {
		out = append(out, domain.Role(role))
	}
	return out
}

func toUser(u repository.IdentityUser, roles []domain.Role) *domain.User {
	return &domain.User{
		ID:        u.ID,
		AuthID:    u.AuthID.String,
		Email:     u.Email,
		Name:      u.Name.String,
		Status:    domain.Status(u.Status),
		Roles:     roles,
		CreatedAt: u.CreatedAt.Time,
		UpdatedAt: u.UpdatedAt.Time,
	}
}
