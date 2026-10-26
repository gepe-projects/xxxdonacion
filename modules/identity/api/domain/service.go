package domain

import (
	"context"
	"uuid"

	"xxxdonacion/platform/pagination"
)

// Service adalah kontrak publik module identity untuk module lain.
// Implementasinya ada di internal/service dan tidak boleh di-import module lain.
type Service interface {
	// Sync memprovision user yang login lewat Firebase, idempoten.
	Sync(ctx context.Context, caller Principal) (*User, error)

	// CreateUser membuat akun Firebase sekaligus baris database.
	CreateUser(ctx context.Context, caller Principal, in CreateUserInput) (*User, error)

	// SetUserStatus mengubah status ACTIVE/DISABLED (juga di Firebase).
	SetUserStatus(ctx context.Context, caller Principal, userID uuid.UUID, in UpdateStatusInput) (*User, error)

	// SetUserRoles mengganti seluruh role user.
	SetUserRoles(ctx context.Context, caller Principal, userID uuid.UUID, in UpdateRolesInput) (*User, error)

	// DeleteUser menghapus akun Firebase dan baris database.
	DeleteUser(ctx context.Context, caller Principal, userID uuid.UUID) error

	// ListUsers dipakai admin untuk mencari user yang akan dikelola.
	// cursor nil berarti halaman pertama; cursor berisi ID user terakhir.
	ListUsers(ctx context.Context, caller Principal, limit int, cursor *uuid.UUID) (pagination.Page[User], error)

	// Authorize dipakai module lain untuk mengecek status dan role user.
	// Implementasi wajib cache-first (Redis), fallback ke database.
	// Tanpa roles: cukup memastikan user ACTIVE.
	Authorize(ctx context.Context, uid string, roles ...Role) (*User, error)
}
