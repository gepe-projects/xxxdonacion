# xxxdonacion — Agent Contract

Dokumen ini adalah kontrak kerja untuk agent dan developer di repo ini.
Baca ini sebelum mengubah kode. Aturan di sini **mengalahkan** skill generik
yang terpasang.

> Kode adalah sumber kebenaran. Kalau dokumentasi dan kode berbeda, ikuti kode,
> lalu perbarui dokumentasi di perubahan yang sama.

---

## TL;DR

| Perintah | Fungsi |
|---|---|
| `make dev` | Jalankan server dengan hot reload (`air`) |
| `make run` | Jalankan server tanpa hot reload |
| `make build` | Build binary ke `bin/app` |
| `make test` | Jalankan seluruh test |
| `make fmt` | Format seluruh file Go |
| `make tidy` | Rapikan `go.mod` / `go.sum` |
| `make sqlc` | Generate repository dari query SQL |
| `make migrate-up` | Terapkan migrasi |
| `make migrate-down` | Rollback satu migrasi |
| `make migrate-status` | Status migrasi |
| `make migrate-create name=<nama>` | Buat migrasi baru |

Verifikasi manual:

```bash
gofmt -l .
go vet ./...
go test ./...
go build ./...
```

Tidak ada target `make vet`; jalankan `go vet ./...` langsung.

---

## Peta Repo

| Path | Isi |
|---|---|
| `cmd/app/main.go` | Entry point tipis |
| `cmd/app/bootstrap.go` | Composition root: config, dependency, module, server |
| `modules/<nama>/api/domain/` | Kontrak publik: entity per file (`user.go`, `role.go`, dst) + interface `Service` di `service.go` |
| `modules/<nama>/internal/delivery/http/` | Handler Echo v5 |
| `modules/<nama>/internal/delivery/http/req/` | Struct request HTTP (JSON tag hanya di sini) |
| `modules/<nama>/internal/service/` | Business logic + operasi Firebase Admin module |
| `modules/<nama>/internal/repository/` | **Generated sqlc — jangan diedit** |
| `modules/<nama>/internal/cache/` | Cache module dengan pola `Get`/`Set`/`Invalidate` |
| `modules/<nama>/module.go` | Wiring lokal module + registrasi route user/admin |
| `platform/*` | Infrastruktur bersama (config, logger, server, db, redis, firebase) |
| `platform/apperr/` | Error model + constructor aplikasi |
| `platform/postgres/` | Pool, `errors.go` (SQLSTATE), `type.go` (konversi pgtype) |
| `platform/pagination/` | Cursor pagination `int64` dan `uuid` |
| `platform/firebase/` | Verifikasi token Firebase saja |
| `platform/server/response/` | Envelope response sukses `{"data": ...}` |
| `platform/server/middleware/` | Request ID, access log, auth, error handler |
| `db/postgres/migrations/` | Migrasi Goose |
| `db/postgres/queries/<module>/` | Query sqlc per module |
| `docs/logging.md` | Sumber kebenaran logging |
| `docs/auth.md` | Sumber kebenaran auth, role, dan endpoint identity |
| `.claude/skills/` | Skill generik project (lihat bagian Skill) |

---

## Arsitektur

Alur dependensi:

```text
delivery/http → service → repository (sqlc)
                     ↑
                api/domain
```

Aturan:

- Handler hanya bind request, memetakan ke input domain, memanggil service, dan
  menulis response. **Jangan** taruh business logic atau validasi bisnis di
  handler.
- Struct request HTTP hanya di `internal/delivery/http/req`. Struct input
  service ada di `api/domain` dan **tidak** memakai JSON tag.
- Validasi input adalah method domain (mis. `CreateUserInput.Validate()`), lalu
  dipanggil service. Jangan membuat validator helper terpisah di handler.
- Business logic ada di service.
- Repository berisi kode generated sqlc. Jangan menambah wrapper manual atau
  mengedit file generated.
- `api/domain` adalah kontrak publik module, dipecah per entity (`user.go`,
  `role.go`, dst). `service.go` hanya berisi interface `Service` yang boleh
  dipakai module lain.
- Module lain hanya boleh memakai `api/domain`, bukan package `internal`.
  Module tidak boleh meng-import package `internal` module lain.
- `cmd/app/bootstrap.go` adalah satu-satunya tempat wiring semua module.
- Jangan melakukan JOIN lintas tabel milik module berbeda; panggil service
  module pemilik data, lalu gabungkan hasilnya di layer pemanggil.
- Jangan menulis langsung ke tabel milik module lain.
- Jangan membuat package `util`, `common`, atau `helpers` sebagai tempat
  sampah. Taruh fungsi di package yang memiliki domainnya.

### Service, repository, dan cache

- Constructor service membuat `repository.New(db)` dan cache **sekali**, lalu
  menyimpannya di struct `Service`.
- Jangan memanggil `repository.New(...)` per request. Untuk transaksi gunakan
  `repo.WithTx(tx)`.
- Cache ada di `internal/cache` dengan pola `Get`/`Set`/`Invalidate`.
- Baca cache Redis lebih dulu; database hanya di-hit saat cache miss.
- Redis dianggap trusted: wajib private, ACL kuat, dan tidak diekspos publik.
- `Invalidate` mengembalikan error dan retry. Mutasi admin role/status/delete
  wajib fail-closed kalau invalidasi gagal; jangan menelan error cache untuk
  aksi security. Fail-closed **hanya** berlaku untuk mutasi admin tersebut.
- `Set` boleh log-only karena DB tetap sumber kebenaran, tetapi cache stale
  maksimal TTL.
- Setiap mutasi user/role/status/delete meng-invalidate cache sebelum dan
  sesudah perubahan.
- `/sync` meng-invalidate cache caller sebelum transaksi dan sesudah commit,
  tanpa syarat. `/sync` adalah provisioning, bukan mutasi security, jadi error
  invalidasi cukup di-log (best-effort); jangan fail-closed.
- Cache diisi oleh `Authorize` saat cache miss. Jangan menulis cache dari
  transaksi sync yang bisa selesai belakangan dan menimpa cache baru.

### Auth dan Authz

- Login, registrasi, OAuth, dan refresh token dilakukan Firebase Client SDK.
  Backend hanya memverifikasi ID token.
- `platform/firebase` hanya untuk verifikasi token. Operasi Firebase Admin
  (create/disable/delete user) ada di service module identity, bukan di
  `platform/firebase`.
- `RequireAuth` hanya memverifikasi identitas. Jangan menaruh role di token,
  context, atau middleware auth.
- Module lain yang butuh authz memakai kontrak publik identity:
  `domain.Service.Authorize(ctx, uid, roles...)`. Implementasi wajib Redis-first,
  fallback database. Module lain tidak boleh query tabel `identity.*`.
- Role default user baru adalah `USER`. Tidak ada seed user di migration;
  SUPER_ADMIN pertama dibuat manual dari database.
- Create user admin **tidak menerima role dari input** dan selalu memberi
  `USER`. Perubahan role hanya lewat endpoint roles.
- Create user admin memakai alur C: authorize + validasi, lalu pre-check email
  lewat `GetUserByEmail` **di luar transaksi**. Email yang sudah ada ditolak
  `409` sebelum Firebase disentuh. Buat Firebase user dengan UID dari Firebase,
  lalu dalam satu transaksi database insert baris `id = UUIDv7` dan
  `auth_id = UID Firebase`, grant `USER`, commit.
- Kalau operasi database gagal, rollback dan hapus akun Firebase. Sebelum
  menghapus, cek `GetUserByAuthID`: kalau sudah ada baris yang mereferensikan
  UID itu (mis. `/sync` menang race), jangan hapus. Kegagalan cleanup digabung
  ke error utama dengan `errors.Join`. Pre-check email bersifat best-effort;
  unique constraint email di database tetap penentu.
- `ADMIN` mengelola `USER`; `SUPER_ADMIN` mengelola `USER` dan `ADMIN`. User
  tidak boleh mengubah status/role sendiri, dan `SUPER_ADMIN` tidak bisa diubah
  lewat API.
- Endpoint admin dipisah path (`/identities/admin/...`) dari endpoint user dan
  memakai `RequireAuthRevoked`. Prefix route bukan batas keamanan; otorisasi
  tetap dicek di service.
- `/sync` dan endpoint admin wajib memakai rate limit Redis.
- Detail kontrak ada di `docs/auth.md`.

---

## Aturan Go

- `gofmt` wajib. Jalankan `gofmt -w .` atau `make fmt`.
- `go vet ./...` harus bersih.
- `go test ./...` harus hijau.
- `context.Context` selalu parameter pertama. Jangan menyimpan context di
  dalam struct.
- Gunakan `errors.Is` / `errors.As` untuk memeriksa error chain.
- Bungkus error dengan konteks: `fmt.Errorf("get user by id: %w", err)`.
- Jangan menelan error dengan `_` kecuali benar-benar tidak ada tindakan yang
  berguna (mis. `Close()` saat cleanup); beri komentar kalau perlu.
- Jangan memakai `panic` untuk kondisi yang bisa ditangani.
- Gunakan `slices` dan `maps` dari standard library bila cocok.
- Jangan menambah dependency hanya karena skill generik menyarankan. Tanya
  dulu apakah standard library sudah cukup.
- Jangan memakai `os.Getenv` di luar `platform/config`. Semua environment
  masuk lewat struct config.
- Jangan mengembalikan detail internal (SQL, cause, stack) ke client.

### Naming

- MixedCaps, bukan snake_case, kecuali file dan test case.
- Hindari stutter: `user.New()`, bukan `user.NewUser()`.
- Initialism tetap konsisten: `userID`, `authID`, `HTTPClient`, `URL`.
- Nama receiver pendek dan konsisten: `func (s *Service)`, `func (h *Handler)`.
- Nama harus menjelaskan domain, bukan tipe: `userID`, bukan `idString`.
- Untuk detail, gunakan skill `golang-naming`.

### Pointer vs Value

Gunakan pointer hanya kalau ada alasan jelas:

| Situasi | Pilihan |
|---|---|
| Method mengubah state | `*T` |
| `nil` bermakna (optional/absent) | `*T` |
| Struct besar, atau berisi `sync.Mutex` | `*T` |
| Tipe kecil dan read-only (`string`, `int`, `time.Time`) | `T` |
| Slice, map, channel, function | bukan pointer |
| Interface | bukan `*interface` |
| Dependency/identity (`*pgxpool.Pool`, `*redis.Client`, `*slog.Logger`) | pointer |

Aturan tambahan:

- Jangan memakai `*pgtype.Text` / `*pgtype.UUID` hanya untuk nullable;
  `pgtype.Text` / `pgtype.UUID` sudah mewakili null.
- Gunakan helper `platform/postgres/type.go` untuk konversi nullable, bukan
  helper lokal di service.
- Jangan mengembalikan `(nil, nil)`. Gunakan error yang jelas.
- Jangan memakai pointer hanya untuk "menghemat copy"; ukur dulu.
- Receiver harus konsisten: kalau satu method memakai pointer receiver,
  biasanya semua method tipe itu pointer.
- Jangan mengubah signature hanya demi mengikuti skill generik kalau pola repo
  sudah jelas.
- File generated sqlc dikecualikan dari aturan pointer/style.

---

## Error, Response, dan Logging

- Error aplikasi memakai `platform/apperr`.
- Gunakan constructor: `apperr.BadRequest`, `apperr.Unauthorized`,
  `apperr.Forbidden`, `apperr.NotFound`, `apperr.Conflict`, dan
  `apperr.Internal(err, "aksi")`.
- Jangan membuat helper error lokal seperti `internalErr`, `forbidden`, atau
  `conflict` di service/module. Kalau dipakai lintas module, taruh di `apperr`.
- `apperr.Wrap(err, kind, message)` hanya untuk kasus yang butuh kind/message
  custom di luar constructor.
- Di handler, cukup `return err`; `middleware.ErrorHandler` yang memetakan ke
  response JSON.
- Semua response sukses wajib memakai `platform/server/response`:
  `{"data": ...}`. `data` boleh object, array, atau `null` (mis. delete).
- Error response tetap `{"message": "..."}` dan
  `{"message": "...", "errors": [...]}` untuk validasi. Jangan menambah field
  baru tanpa alasan.
- Pesan user-facing berbahasa Inggris. Komentar dan pesan internal boleh
  bahasa Indonesia.
- Satu error ditangani sekali: **log atau return, jangan keduanya**.
- `AccessLog` mencatat satu baris per request, termasuk error. Jangan menambah
  log error lagi di handler/service.
- Gunakan `slog` dengan context: `slog.InfoContext(ctx, ...)`,
  `slog.ErrorContext(ctx, ...)`.
- `request_id` dan `auth_uid` disuntikkan otomatis oleh logger handler dari
  context. Jangan menambahkannya manual.
- `logger.FromContext(ctx)` mengembalikan logger yang disimpan di context, atau
  `slog.Default()` kalau tidak ada. Aman dipakai untuk menambah attribute.
- Jangan log token, password, secret, authorization header, cookie, credential,
  atau private key.
- `RequireAuth` hanya memverifikasi identitas, bukan otorisasi. Authorization
  dicek di service/handler, dan module lain memakai `domain.Service.Authorize`.
- Untuk detail logging, baca `docs/logging.md`; jangan menduplikasi isinya di
  sini.

---

## SQL, sqlc, dan Migrasi

- Migrasi memakai Goose di `db/postgres/migrations/`.
- Query memakai sqlc di `db/postgres/queries/<module>/`.
- Satu module punya satu entry di `sqlc.yaml`.
- Jangan mengedit file generated: `repository/db.go`, `repository/models.go`,
  `repository/*.sql.go`.
- Setelah mengubah query, jalankan `make sqlc`.
- Setelah mengubah schema, buat migrasi dengan
  `make migrate-create name=<nama>`, isi `-- +goose Up` dan `-- +goose Down`,
  lalu `make migrate-up`.
- Jangan mengubah migrasi yang sudah di-apply di environment bersama.
- Jangan memakai ORM atau query builder baru tanpa diskusi.
- Cek error database lewat `platform/postgres/errors.go`:
  `postgres.IsNoRows`, `postgres.IsUniqueViolation`,
  `postgres.IsForeignKeyViolation`, dst. Jangan cek `pgconn.PgError` atau
  `pgx.ErrNoRows` langsung di service.
- Konversi nilai ke/dari `pgtype` lewat `platform/postgres/type.go`:
  `postgres.ToPgText`, `postgres.ToPgTextPtr`, `postgres.ToPgBool`,
  `postgres.ToPgInt4`, `postgres.ToPgInt8`, `postgres.ToPgUUID`, dan `From*`.
  Jangan membuat helper lokal seperti `nameValue` atau cast pgtype manual.
- Cursor pagination memakai `platform/pagination`:
  `EncodeInt64`/`DecodeInt64` atau `EncodeUUID`/`DecodeUUID`, `ParseLimit`,
  `Page[T]`, dan `NewPage`.
- Query keyset mengambil `limit+1` baris, lalu `NewPage` memotong dan mengisi
  `next_cursor`. Jangan kembali ke offset pagination untuk list baru.
- Untuk list yang diurutkan berdasarkan `id` UUID, gunakan `uuid.NewV7()` agar
  urutan ID mengikuti waktu.

---

## Testing

- Gunakan standard library `testing`; jangan tambah framework test tanpa
  alasan kuat.
- Test table-driven untuk beberapa kasus.
- Gunakan `t.Helper()` di helper test.
- Gunakan `httptest` untuk handler/middleware.
- Assert behavior dan status code, bukan string message yang panjang.
- Test validasi domain di package `api/domain`, bukan lewat handler.
- Test helper platform (`apperr`, `postgres`, `pagination`, `response`) di
  package masing-masing.
- Untuk perubahan yang menyentuh error/logging, tambahkan test di package yang
  relevan.
- Jangan memakai `time.Sleep` untuk memajukan waktu; gunakan mekanisme yang
  bisa dikontrol.
- Test tidak boleh bergantung pada database/Redis nyata kecuali memang
  integration test dan sudah disepakati.

---

## Prinsip Perubahan Minimal

Ponytail adalah default perilaku untuk semua task coding. Batasan project di
bagian ini tetap mengikat dan menang atas preferensi skill:

- Baca kode, test, dan semua caller sebelum mengubah.
- Pakai helper, komponen, atau pola yang sudah ada di repo.
- Ambil perubahan terkecil yang menyelesaikan seluruh task.
- Jangan menambah abstraksi, opsi, config, atau dependency untuk kebutuhan
  hipotetis.
- Jangan refactor kode yang tidak terkait task.
- Jangan memotong validasi, error handling, security, atau test.
- Jangan menghapus aturan repo (generated files, boundary module, `apperr`)
  demi diff yang lebih kecil.
- Sebelum menghapus kode yang terlihat tidak terpakai, cek caller, test,
  konfigurasi, dan referensi string/dinamis.
- Kalau meninggalkan shortcut yang disengaja, tulis komentar:
  `shortcut: <batasannya>, <kapan harus diperbaiki>`.
- Akhiri pekerjaan dengan menyebut apa yang belum dicek dan risiko yang perlu
  diketahui.

Gunakan level Ponytail `full` sebagai default. Jangan mengganti ke `lite` atau
`ultra` tanpa permintaan eksplisit.

---

## Skill Generik yang Terpasang

Skill berikut bersifat generik dan bisa dipakai saat relevan:

- `ponytail` — smallest complete change, reuse, hindari overengineering.
- `ponytail-review` — review perubahan/diff dari sisi bug, risk, test, dan bloat.
- `ponytail-audit` — audit seluruh repo untuk bug, risk, test gap, dan kode
  yang bisa dihapus/disederhanakan.
- `golang-code-style` — readability, control flow, value vs pointer argument.
- `golang-structs-interfaces` — pointer receiver, interface, struct tag.
- `golang-error-handling` — wrapping, `errors.Is/As`, panic/recover, `slog`.
- `golang-naming` — nama package, tipe, function, test.
- `grill-with-docs` — interview desain bertahap; komposisi dari `grilling` +
  `domain-modeling`.
- `grilling` — mesin interview satu-pertanyaan-per-giliran.
- `domain-modeling` — glosarium domain dan ADR.

Skill desain di atas default menulis `GLOSSARY.md` di root dan ADR di
`docs/adr/`. Jangan sampai membuat sumber kebenaran kedua: ikuti lokasi `docs/`
yang sudah ada, dan aturan `AGENTS.md` tetap menang.

Kalau skill bertentangan dengan `AGENTS.md`, **`AGENTS.md` menang**. Beberapa
contoh konflik yang harus di-override:

- Skill menyarankan `samber/oops`; repo ini memakai `apperr`. Jangan tambah
  `samber/oops`.
- Skill menyarankan slice/map selalu diinisialisasi; kode generated sqlc
  dikecualikan.
- Skill menyarankan interface di sisi consumer; `domain.Service` yang sudah ada
  adalah kontrak publik module. Jangan menghapusnya tanpa alasan.
- Jangan mengubah `handler.New` dari concrete `*service.Service` ke interface
  hanya demi mengikuti skill.
- Jangan menambah dependency, logger, atau middleware baru hanya karena skill
  menyebutkannya.
- Jangan membuat abstraksi Firebase Admin di platform; `*auth.Client` tetap
  concrete. Interface kecil `firebaseAdmin` di
  `modules/identity/internal/service` diperbolehkan sebagai test seam, dan
  `*auth.Client` memenuhinya tanpa adapter.
- Jangan membuat helper error lokal di module; pakai constructor `apperr`.
- Jangan mengganti envelope response sukses `{"data": ...}` dengan bentuk lain
  tanpa diskusi.
- Ponytail tidak boleh menghapus validasi, error handling, security, atau test.
  Aturan repo tetap berlaku meskipun diff menjadi lebih besar.

---

## Checklist Sebelum Selesai

- [ ] `gofmt -l .` tidak menghasilkan output.
- [ ] `go vet ./...` bersih.
- [ ] `go test ./...` hijau.
- [ ] `go build ./...` sukses.
- [ ] Tidak ada file generated sqlc yang diedit.
- [ ] Tidak ada `os.Getenv` baru di luar `platform/config`.
- [ ] Tidak ada secret/token yang di-log.
- [ ] `.env` dan file service account mode 600; secret production lewat secret manager.
- [ ] Error baru memakai constructor `apperr` dan pesan user-facing bahasa Inggris.
- [ ] Tidak ada helper error lokal (`internalErr`, `forbidden`, dst) di service.
- [ ] Response sukses baru memakai envelope `platform/server/response`.
- [ ] Kontrak domain dipecah per entity; `service.go` hanya interface.
- [ ] Request struct HTTP ada di `delivery/http/req`; input domain tanpa JSON tag.
- [ ] Validasi input ada di method domain, bukan di handler/service helper.
- [ ] Endpoint admin dipisah path dan otorisasi tetap dicek di service.
- [ ] Repository dan cache dibuat sekali di constructor service; transaksi pakai `repo.WithTx`.
- [ ] Error/pgtype Postgres memakai helper `platform/postgres`.
- [ ] Authz module lain memakai `domain.Service.Authorize` (Redis-first), bukan query tabel identity.
- [ ] Mutasi admin role/status/delete meng-invalidate cache sebelum dan sesudah perubahan; kegagalan invalidasi fail-closed.
- [ ] `/sync` meng-invalidate cache caller sebelum transaksi dan sesudah commit; error invalidasi log-only.
- [ ] Create user admin memakai alur C (pre-check email best-effort, UID dari Firebase, insert `UUIDv7` + `auth_id` dalam transaksi, cleanup Firebase dengan guard `GetUserByAuthID`) dan selalu `USER`.
- [ ] Endpoint admin memakai `RequireAuthRevoked` dan rate limit Redis.
- [ ] Cursor pagination baru memakai `platform/pagination`.
- [ ] Operasi Firebase Admin ada di service module, bukan `platform/firebase`.
- [ ] Config menolak `APP_ENV` kosong/tidak valid; cek kredensial default hanya berlaku di staging/production (`APP_ENV=test` diperlakukan seperti development).
- [ ] Handler tetap tipis; business logic ada di service.
- [ ] Perubahan tidak melebar ke kode yang tidak terkait.
- [ ] Dokumentasi diperbarui kalau perilaku berubah.
