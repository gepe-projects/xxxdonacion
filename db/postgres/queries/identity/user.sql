-- name: GetUserByAuthID :one
SELECT id, auth_id, email, name, status, created_at, updated_at
FROM identity.users
WHERE auth_id = $1;

-- name: GetUserByID :one
SELECT id, auth_id, email, name, status, created_at, updated_at
FROM identity.users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, auth_id, email, name, status, created_at, updated_at
FROM identity.users
WHERE email = $1;

-- Idempoten: ON CONFLICT DO NOTHING -> kalau bentrok, RETURNING kosong
-- (pgx.ErrNoRows) dan pemanggil re-select.
-- name: CreateUserIfAbsent :one
INSERT INTO identity.users (id, auth_id, email, name, status)
VALUES ($1, $2, $3, $4, $5)
    ON CONFLICT DO NOTHING
RETURNING id, auth_id, email, name, status, created_at, updated_at;

-- Link akun Firebase ke baris yang sudah ada (mis. SUPER_ADMIN manual).
-- auth_id IS NULL membuat operasi ini idempoten: kalau sudah di-link, RETURNING kosong.
-- name: LinkUserAuthID :one
UPDATE identity.users
SET auth_id    = $2,
    updated_at = now()
WHERE id = $1
  AND auth_id IS NULL
RETURNING id, auth_id, email, name, status, created_at, updated_at;

-- name: UpdateUserProfile :one
UPDATE identity.users
SET email      = $2,
    name       = $3,
    updated_at = now()
WHERE id = $1
RETURNING id, auth_id, email, name, status, created_at, updated_at;

-- name: UpdateUserStatus :one
UPDATE identity.users
SET status     = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, auth_id, email, name, status, created_at, updated_at;

-- name: DeleteUser :exec
DELETE FROM identity.users
WHERE id = $1;

-- name: ListUsers :many
SELECT u.id,
       u.auth_id,
       u.email,
       u.name,
       u.status,
       u.created_at,
       u.updated_at,
       COALESCE(array_agg(r.role ORDER BY r.role) FILTER (WHERE r.role IS NOT NULL), '{}')::text[] AS roles
FROM identity.users u
         LEFT JOIN identity.user_roles r ON r.user_id = u.id
GROUP BY u.id
ORDER BY u.id DESC
LIMIT $1;

-- name: ListUsersAfter :many
SELECT u.id,
       u.auth_id,
       u.email,
       u.name,
       u.status,
       u.created_at,
       u.updated_at,
       COALESCE(array_agg(r.role ORDER BY r.role) FILTER (WHERE r.role IS NOT NULL), '{}')::text[] AS roles
FROM identity.users u
         LEFT JOIN identity.user_roles r ON r.user_id = u.id
WHERE u.id < $1
GROUP BY u.id
ORDER BY u.id DESC
LIMIT $2;