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
| `modules/<nama>/api/domain/` | Kontrak publik module (entity + interface `Service`) |
| `modules/<nama>/internal/delivery/http/` | Handler Echo v5 |
| `modules/<nama>/internal/service/` | Business logic |
| `modules/<nama>/internal/repository/` | **Generated sqlc — jangan diedit** |
| `modules/<nama>/internal/cache/` | Cache module (kalau ada) |
| `modules/<nama>/module.go` | Wiring lokal module + registrasi route |
| `platform/*` | Infrastruktur bersama (config, logger, server, db, redis, firebase) |
| `platform/apperr/` | Error model aplikasi |
| `platform/server/middleware/` | Request ID, access log, auth, error handler |
| `db/postgres/migrations/` | Migrasi Goose |
| `db/postgres/queries/<module>/` | Query sqlc per module |
| `docs/logging.md` | Sumber kebenaran logging |
| `.opencode/skills/` | Skill Go generik (lihat bagian Skill) |

---

## Arsitektur

Alur dependensi:

```text
delivery/http → service → repository (sqlc)
                     ↑
                api/domain
```

Aturan:

- Handler hanya parsing, validasi, memanggil service, dan menulis response.
  **Jangan** taruh business logic di handler.
- Business logic ada di service.
- Repository berisi kode generated sqlc. Jangan menambah wrapper manual atau
  mengedit file generated.
- `api/domain` adalah kontrak publik module. Module lain hanya boleh memakai
  kontrak ini, bukan package `internal`.
- Module tidak boleh meng-import package `internal` module lain.
- `cmd/app/bootstrap.go` adalah satu-satunya tempat wiring semua module.
- Jangan melakukan JOIN lintas tabel milik module berbeda; panggil service
  module pemilik data, lalu gabungkan hasilnya di layer pemanggil.
- Jangan menulis langsung ke tabel milik module lain.
- Jangan membuat package `util`, `common`, atau `helpers` sebagai tempat
  sampah. Taruh fungsi di package yang memiliki domainnya.

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
- Di service, bungkus cause dengan `apperr.Wrap(err, kind, "pesan user")`.
- Di handler, cukup `return err`; `middleware.ErrorHandler` yang memetakan ke
  response JSON.
- Bentuk response saat ini: `{"message": "..."}` dan
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
  dicek di service/handler sesuai kebutuhan.
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

---

## Testing

- Gunakan standard library `testing`; jangan tambah framework test tanpa
  alasan kuat.
- Test table-driven untuk beberapa kasus.
- Gunakan `t.Helper()` di helper test.
- Gunakan `httptest` untuk handler/middleware.
- Assert behavior dan status code, bukan string message yang panjang.
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
- [ ] Error baru memakai `apperr` dan pesan user-facing bahasa Inggris.
- [ ] Handler tetap tipis; business logic ada di service.
- [ ] Perubahan tidak melebar ke kode yang tidak terkait.
- [ ] Dokumentasi diperbarui kalau perilaku berubah.
