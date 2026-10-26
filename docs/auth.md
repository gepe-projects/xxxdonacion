# Auth dan Authz

Sumber kebenaran untuk alur autentikasi (Firebase) dan otorisasi (role di
backend). Baca ini sebelum mengubah endpoint identity.

## Prinsip

- Login, registrasi, reset password, dan refresh token **dilakukan di client**
  memakai Firebase Client SDK. Backend tidak menerima password untuk login;
  password hanya bisa di-set admin saat membuat akun.
- Backend hanya **memverifikasi ID token** dan menyimpan profil + role di
  database.
- Role **tidak pernah** berasal dari client atau custom claim Firebase. Role
  selalu dibaca dari `identity.user_roles`.
- Default role untuk user baru adalah `USER`.
- `SUPER_ADMIN` pertama dibuat manual dari database. Tidak ada seed user di
  migration. Role ini tidak bisa diberikan, diubah, atau dihapus lewat API.
- `platform/firebase` hanya menangani verifikasi token. Operasi Firebase Admin
  (create/disable/delete) ada di `modules/identity/internal/service/firebase.go`.
- Struct request HTTP ada di delivery (`createUserRequest`, dst). Struct input
  service ada di domain (`CreateUserInput`, dst). Handler memetakan request ke
  input domain.
- Validasi input ada di method domain (`CreateUserInput.Validate`,
  `UpdateStatusInput.Validate`, `UpdateRolesInput.Validate`), bukan di handler.
- Semua response sukses memakai envelope `{"data": ...}` dari
  `platform/server/response`.

## Verifikasi token

`platform/server/middleware/requireAuth.go`:

1. Ambil `Authorization: Bearer <id_token>`.
2. `firebase.Client.Verify` memanggil `VerifyIDToken` dari Admin SDK.
3. Identity disimpan di context dan `auth_uid` disuntikkan ke log.

Admin SDK memverifikasi signature, `iss`, `aud`, `exp`, dan `sub` secara lokal.
Public key Google di-cache di memori mengikuti header `cache-control` (sekitar
satu jam) dan di-refresh saat `kid` tidak dikenal. Jadi tidak ada network call
per request dan tidak perlu cache JWKS sendiri.

Konsekuensi:

- Tiap instance punya cache sendiri; cold start melakukan satu fetch.
- Token user yang di-revoke/disable tetap valid sampai expired (maksimal sekitar
  satu jam). Karena itu backend mengecek `status` user di database pada operasi
  yang membutuhkan otorisasi, bukan hanya mengandalkan token.
- Jangan memakai `VerifyIDTokenAndCheckRevoked` per request; fungsi itu memanggil
  server Google setiap kali.

## Endpoint

Endpoint user dan admin dipisah path. Semua membutuhkan Bearer ID token.

**User**

| Method | Path | Fungsi |
|---|---|---|
| POST | `/identities/sync` | Provision/link user dari token Firebase |
| GET | `/identities/me` | Identity dari token + cek status aktif user |

**Admin** (`ADMIN`/`SUPER_ADMIN`)

| Method | Path | Fungsi |
|---|---|---|
| POST | `/identities/admin/users` | Buat user di Firebase + database |
| GET | `/identities/admin/users` | List user (`limit`, `offset`) |
| PATCH | `/identities/admin/users/:id/status` | Set `ACTIVE`/`DISABLED` |
| PUT | `/identities/admin/users/:id/roles` | Ganti seluruh role user |
| DELETE | `/identities/admin/users/:id` | Hapus user di Firebase + database |

User biasa tidak punya endpoint create user. Pendaftaran mandiri terjadi di
Firebase Client SDK; backend hanya memprovision lewat `/sync`.

### POST /identities/sync

Client memanggil ini setelah login/daftar (dan boleh lagi saat aplikasi dibuka).

Syarat: `email` tidak kosong dan `email_verified = true` di token.

Alur dalam satu transaksi:

1. Cari `auth_id = uid`. Kalau ada, cek status lalu update email/name kalau
   berubah.
2. Kalau tidak ada, cari `email`:
   - ada dan `auth_id` masih NULL → link `auth_id` (role lama dipertahankan,
     termasuk SUPER_ADMIN yang dibuat manual);
   - ada dan `auth_id` milik akun lain → `409`;
   - tidak ada → buat user `ACTIVE` + role `USER`.
3. Kembalikan profil dan role terbaru.

Operasi idempoten dan menangani race lewat unique constraint + re-select.

Response `200`:

```json
{
  "data": {
    "id": "0199...",
    "email": "user@example.com",
    "name": "User",
    "status": "ACTIVE",
    "roles": ["USER"]
  }
}
```

### GET /identities/me

Mengembalikan identity dari token, tetapi mengecek status user lewat
`Authorize` (cache Redis dulu, fallback database). User disabled atau belum
ter-provision akan mendapat `403`.

```json
{
  "data": {
    "uid": "firebase-uid",
    "email": "user@example.com",
    "name": "User",
    "email_verified": true
  }
}
```

### POST /identities/admin/users

Membuat akun Firebase (Admin SDK) sekaligus baris database. Email dianggap
terverifikasi karena dibuat oleh admin. **Tidak ada role dari input**; user baru
selalu mendapat role `USER`. Role lain diatur lewat endpoint roles.

Request:

```json
{
  "email": "user@example.com",
  "name": "User",
  "password": "strongpassword"
}
```

- `password` opsional. Kalau diisi minimal 12 karakter. Kalau kosong, akun
  Firebase dibuat tanpa password (user masuk lewat OAuth atau reset password).
- Urutan create: baris database (UID dibuat sendiri) → akun Firebase dengan UID
  yang sama → commit. Kalau email sudah ada, Firebase tidak disentuh.
- Kalau Firebase gagal, transaksi database di-rollback. Kalau commit gagal,
  akun Firebase dihapus; kalau cleanup gagal, akun itu tidak punya baris DB
  privileged dan `/sync` hanya akan membuat user `USER`.

Response `201` memakai envelope `{"data": {...user...}}`.

### PATCH /identities/admin/users/:id/status

```json
{"status": "DISABLED"}
```

Status diubah di Firebase **dan** database. Kalau update database gagal, status
Firebase dikembalikan ke nilai sebelumnya.

- Tidak bisa mengubah status sendiri.
- `SUPER_ADMIN` tidak bisa diubah lewat API.
- `ADMIN` hanya bisa mengubah status `USER`.

### PUT /identities/admin/users/:id/roles

```json
{"roles": ["USER", "ADMIN"]}
```

Mengganti seluruh role user. Role yang dikirim harus `USER` atau `ADMIN`;
`SUPER_ADMIN` ditolak.

- Tidak bisa mengubah role sendiri.
- `SUPER_ADMIN` tidak bisa diubah lewat API.
- `ADMIN` hanya bisa mengelola role `USER`.
- `SUPER_ADMIN` bisa mengelola `USER` dan `ADMIN`.

### DELETE /identities/admin/users/:id

Menghapus akun Firebase lalu baris database. `deleteFirebaseUser` idempoten
(user yang sudah tidak ada dianggap sukses), jadi retry setelah kegagalan
database aman.

- Tidak bisa menghapus diri sendiri.
- `SUPER_ADMIN` tidak bisa dihapus lewat API.
- `ADMIN` hanya bisa menghapus `USER`.

Response `200`:

```json
{"data": null}
```

### GET /identities/admin/users

Cursor pagination:

- `limit` opsional, default 50, maksimal 100.
- `cursor` opsional, ambil dari `next_cursor` response sebelumnya.
- Urutan: `id DESC` (UUIDv7 untuk user baru).

Response:

```json
{
  "data": {
    "items": [
      {
        "id": "0199...",
        "email": "user@example.com",
        "name": "User",
        "status": "ACTIVE",
        "roles": ["USER"]
      }
    ],
    "next_cursor": "dTpw...",
    "has_more": true
  }
}
```

Kalau `has_more` false, `next_cursor` tidak dikirim.

## Bootstrap super admin

Tidak ada seed user di migration. Setelah user pertama login lewat Firebase dan
memanggil `/sync` (otomatis dapat role `USER`), jadikan dia super admin manual:

```sql
UPDATE identity.user_roles
SET role = 'SUPER_ADMIN'
WHERE user_id = (SELECT id FROM identity.users WHERE email = 'admin@example.com');
```

Alternatif: insert user langsung dari database dengan `auth_id` dari Firebase
Console:

```sql
INSERT INTO identity.users (id, auth_id, email, name, status)
VALUES (gen_random_uuid(), '<firebase-uid>', 'admin@example.com', 'Super Admin', 'ACTIVE');

INSERT INTO identity.user_roles (user_id, role)
SELECT id, 'SUPER_ADMIN' FROM identity.users WHERE auth_id = '<firebase-uid>';
```

## Cursor pagination

`platform/pagination` menyediakan cursor opaque untuk primary key `BIGINT`
maupun `UUID`:

```go
pagination.EncodeInt64(id)
pagination.DecodeInt64(cursor)

pagination.EncodeUUID(id)
pagination.DecodeUUID(cursor)
```

`cursor` adalah token opaque (base64 URL-safe) yang membawa tipe dan nilai ID,
bukan ID mentah, sehingga client tidak perlu tahu tipe primary key.
`pagination.ParseLimit` menangani default
dan batas maksimum, sedangkan `pagination.Page[T]` adalah bentuk response
standar (`items`, `next_cursor`, `has_more`).

Pemakaian di query: ambil `limit+1` baris, lalu `pagination.NewPage` memotong
ke `limit` dan mengisi `next_cursor` dari item terakhir.

## Cache identity

`modules/identity/internal/cache/user.go` menyimpan `domain.User` (termasuk
role dan status) di Redis dengan key `identity:user:<firebase_uid>`, TTL 10
menit.

- Redis dianggap trusted: harus private, ACL kuat, dan tidak diekspos ke publik.
- `Authorize` (dipakai admin dan module lain) memakai cache lebih dulu, lalu
  fallback ke database dan mengisi cache.
- Perubahan role, status, dan delete meng-invalidate cache sebelum dan sesudah
  mutasi.
- `Invalidate` retry tiga kali. Kalau tetap gagal, mutasi security mengembalikan
  error (fail-closed), supaya perubahan tidak diam-diam tertinggal di cache.
- `/sync` tidak menulis cache; ini menghindari sync yang selesai belakangan
  menimpa cache baru setelah admin mengubah role/status.
- Perubahan langsung di database di luar aplikasi bisa terlihat stale maksimal
  10 menit.

## Rate limit

- `/identities/sync`: 10 request per menit per UID.
- `/identities/admin/*`: 60 request per menit per UID.
- Memakai Redis, jadi berlaku lintas instance. Response `429` menyertakan
  `Retry-After`.
- Tidak ada rate limit per IP di aplikasi karena CGNAT/NAT kantor bisa membuat
  banyak user berbagi IP. Rate limit per IP, kalau perlu, taruh di reverse
  proxy/edge untuk proteksi volumetrik.
- Kalau Redis bermasalah, limiter fail-open dan error hanya di-log supaya API
  tetap hidup.

## Dipakai module lain

`RequireAuth` tetap hanya menyediakan identity Firebase (UID). Module lain yang
butuh status/role user memanggil `Authorize` dari kontrak publik identity:

```go
id, ok := middleware.Identity(c.Request().Context())
if !ok {
    return apperr.Unauthorized("unauthenticated")
}

user, err := identityService.Authorize(
    ctx,
    id.UID,
    identitydomain.RoleAdmin,
    identitydomain.RoleSuperAdmin,
)
```

- `Authorize` mengecek Redis dulu; database hanya di-hit saat cache miss.
- Tanpa variadic role, `Authorize` hanya memastikan user `ACTIVE`.
- User disabled atau role tidak cocok menghasilkan `403` melalui `apperr`.
- Module lain menerima `identity domain.Service` lewat constructor/Dependencies
  di bootstrap. Jangan import package `internal` identity atau query tabel
  `identity.*` langsung.
- `Module.Services()` mengembalikan `domain.Service` untuk module lain yang
  butuh kontrak ini.

## Status dan error

| Status | Arti |
|---|---|
| 400 | Body/role/status/password tidak valid |
| 401 | Bearer token hilang, invalid, atau expired |
| 403 | Email belum verified, akun disabled, bukan admin, atau target tidak boleh diubah |
| 404 | User target tidak ditemukan |
| 409 | Email sudah dipakai/ter-link ke akun lain, atau konflik database |
| 429 | Rate limit terlampaui |
| 500 | Error internal |

Pesan user-facing memakai bahasa Inggris dan dipetakan oleh
`platform/server/middleware/error.go`.

## Catatan implementasi

- `/sync`, create user, dan perubahan role memakai transaksi database.
- Repository (`repository.Queries`) dibuat sekali di `service.New`; transaksi
  memakai `repo.WithTx(tx)`.
- `ADMIN`/`SUPER_ADMIN` dicek di service, bukan di middleware. Endpoint admin
  memakai `RequireAuthRevoked` sehingga token yang di-revoke/disable ditolak.
- Endpoint user memakai `RequireAuth` lokal. `/me` mengecek status user lewat
  `Authorize` (cache/DB), jadi user disabled langsung ditolak.
- Config production memvalidasi `APP_ENV`, kredensial database default, password
  Redis default, dan wildcard CORS. `sslmode=disable` tetap diizinkan untuk
  jaringan private Docker Compose.
- CORS hanya aktif kalau `APP_CORS_ORIGINS` diisi, dan memakai allowlist
  (bukan wildcard).

## Deployment

Server listen di `:APP_PORT` pada semua interface. Jangan ekspos port ini
langsung ke internet: pakai firewall dan reverse proxy yang menangani TLS.
Database dan Redis harus berada di network private, bukan port publik.
