-- +goose Up
CREATE SCHEMA IF NOT EXISTS identity;
CREATE EXTENSION IF NOT EXISTS citext;

-- identity.users
-- auth_id NULL = belum pernah login / belum di-link ke akun Firebase.
-- name NULLABLE: Firebase tidak menjamin displayName ada (mis. email/password).
CREATE TABLE identity.users
(
    id         UUID PRIMARY KEY,
    auth_id    VARCHAR(128) UNIQUE,
    email      citext       NOT NULL,
    name       VARCHAR(100),
    status     VARCHAR(20) NOT NULL, -- ACTIVE / DISABLED
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_identity_users_email ON identity.users (email);
CREATE INDEX ix_identity_users_created_at
    ON identity.users (created_at DESC, id DESC);

CREATE TABLE identity.user_roles
(
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES identity.users (id) ON DELETE CASCADE,
    role       VARCHAR(30) NOT NULL, -- SUPER_ADMIN / ADMIN / USER
    granted_by UUID,                 -- null = self (auto-provision) / system
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ux_identity_user_role UNIQUE (user_id, role)
);
CREATE INDEX ix_identity_user_roles_role
    ON identity.user_roles (role, user_id);

-- Tidak ada seed user. SUPER_ADMIN pertama dibuat manual dari database setelah
-- login Firebase pertama (lihat docs/auth.md bagian bootstrap super admin).

-- +goose Down
-- Semua objek migration ini hidup di schema identity, jadi cukup drop schema-nya
-- (CASCADE menghapus tabel, index, dan identity sequence sekaligus).
DROP SCHEMA IF EXISTS identity CASCADE;
-- citext sengaja TIDAK di-drop: extension level-database, Up memakai
-- IF NOT EXISTS (aman diulang), dan drop bisa mengganggu objek lain yang memakainya.
