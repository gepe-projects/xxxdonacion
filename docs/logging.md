# Logging

Dokumen ini menjelaskan alur logging di aplikasi ini dari startup sampai satu
request selesai, termasuk cara pakai, aturan, dan kesiapan integrasi dengan
Grafana Alloy/Loki.

Prinsip utamanya:

- **Satu logger** (`log/slog`) untuk seluruh aplikasi.
- **Satu handler**: format, level, dan tujuan output ditentukan di satu tempat.
- **JSON satu baris ke stdout** di luar development.
- **`request_id` otomatis** masuk ke semua log yang membawa context.
- **Satu error request dicatat satu kali**, yaitu di `AccessLog`.

---

## Daftar isi

1. [Peta file](#1-peta-file)
2. [Startup: logger dibuat sekali](#2-startup-logger-dibuat-sekali)
3. [Alur satu request](#3-alur-satu-request)
4. [Bagaimana `request_id` masuk ke log](#4-bagaimana-request_id-masuk-ke-log)
5. [Bagaimana `user_id` masuk](#5-bagaimana-user_id-masuk)
6. [Isi access log](#6-isi-access-log)
7. [Alur error: kenapa tidak dobel](#7-alur-error-kenapa-tidak-dobel)
8. [Alur panic](#8-alur-panic)
9. [Cara pakai sehari-hari](#9-cara-pakai-sehari-hari)
10. [Konfigurasi environment](#10-konfigurasi-environment)
11. [Kesiapan Grafana Alloy/Loki](#11-kesiapan-grafana-alloyloki)
12. [Cara memverifikasi sendiri](#12-cara-memverifikasi-sendiri)
13. [Kalau mau mengubah sesuatu](#13-kalau-mau-mengubah-sesuatu)
14. [Ringkasan mental model](#14-ringkasan-mental-model)

---

## 1. Peta file

| File | Perannya |
|---|---|
| `cmd/app/main.go` | entry point, log fatal error |
| `cmd/app/bootstrap.go` | init logger, log lifecycle startup/shutdown |
| `platform/config/config.go` | baca `APP_LOG_LEVEL`, `APP_LOG_FORMAT`, dll. |
| `platform/logger/logger.go` | membuat **satu** logger + base attribute |
| `platform/logger/handler.go` | menyuntik `request_id` dari context ke setiap log |
| `platform/logger/context.go` | helper `WithRequestID`, `FromContext`, `WithContext` |
| `platform/server/server.go` | urutan middleware |
| `platform/server/middleware/req_id.go` | membuat/memvalidasi request ID |
| `platform/server/middleware/logger.go` | `AccessLog`, satu baris log per request |
| `platform/server/middleware/requireAuth.go` | menambah `user_id` ke context |
| `platform/server/middleware/error.go` | mengubah error jadi response JSON, **tidak** logging |
| `platform/apperr/apperror.go` | error dengan `Kind`, `Message`, dan `cause` |

---

## 2. Startup: logger dibuat sekali

Alurnya:

```text
main()
  └─ run()
       ├─ loadConfig()
       ├─ logger.New(cfg)   ← di sini satu-satunya slog.New()
       └─ serve(cfg, log)
            ├─ postgres.New(...)
            ├─ redis.New(...)
            ├─ server.New(cfg)
            └─ srv.Start()
```

Di `cmd/app/bootstrap.go`:

```go
func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logger.New(cfg)

	return serve(cfg, log)
}
```

`logger.New` melakukan lima hal penting:

```go
func NewWithWriter(cfg *config.App, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     parseLevel(cfg.LogLevel, cfg.Environment),
		AddSource: isDevelopment(cfg.Environment),
	}

	var h slog.Handler
	if format(cfg.LogFormat, cfg.Environment) == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}

	// Membungkus handler supaya request_id dari context ikut masuk.
	h = &contextHandler{Handler: h}

	base := []any{
		"service", cfg.Name,
		"env", cfg.Environment,
	}
	if version, commit := buildInfo(); version != "" {
		// Prefix service_ supaya tidak bentrok dengan field "version"
		// yang ditulis library lain (Echo menulis version saat startup).
		base = append(base, "service_version", version)
		if commit != "" {
			base = append(base, "service_commit", commit)
		}
	}

	log := slog.New(h).With(base...)
	slog.SetDefault(log)
	return log
}
```

Penjelasannya:

- **`opts.Level`** menentukan level minimum. Kosong = `debug` di development,
  `info` di environment lain.
- **`AddSource`** hanya aktif di development. Di production dimatikan karena
  setiap log jadi memanggil `runtime.Caller`; mahal kalau traffic tinggi.
- **Handler** dipilih di sini: JSON di luar development, text di development.
- **`contextHandler`** membungkus handler asli. Inilah yang menambahkan
  `request_id` nanti.
- **`base`** adalah attribute yang selalu ada di semua log: `service`, `env`,
  `service_version`, `service_commit`.
- **`slog.SetDefault(log)`** membuat logger ini dipakai seluruh aplikasi,
  termasuk Echo dan library yang memakai `slog.Default()`.

Di `platform/server/server.go`, Echo memakai logger yang sama:

```go
e := echo.NewWithConfig(echo.Config{
	Logger: slog.Default(),
})
```

Jadi log internal Echo juga masuk ke format dan tujuan yang sama, bukan logger
terpisah.

> Kenapa `service_version` bukan `version`? Karena Echo juga menulis field
> `version` saat startup. Kalau namanya sama, JSON punya duplicate key.

---

## 3. Alur satu request

Urutan middleware di `platform/server/server.go`:

```go
e.Use(mw.RequestID())     // 1, paling luar
e.Use(mw.AccessLog())     // 2
e.Use(emw.Recover(...))   // 3
e.Use(emw.BodyLimit(...)) // 4
e.HTTPErrorHandler = mw.ErrorHandler()
```

Di Echo, **middleware yang didaftarkan lebih dulu posisinya lebih luar**.
Alurnya:

```text
Request masuk
   │
   ▼
RequestID
   ├─ baca X-Request-Id dari client
   ├─ validasi; kalau tidak valid → generate UUIDv7
   ├─ set response header X-Request-Id
   └─ simpan request_id ke request context
   │
   ▼
AccessLog
   ├─ catat waktu mulai
   └─ panggil next
        │
        ▼
     Recover
        ├─ defer recover()
        └─ panggil next
             │
             ▼
          BodyLimit
             │
             ▼
          RequireAuth (kalau route memakainya)
             ├─ verifikasi token
             ├─ kalau gagal → return apperr
             └─ kalau sukses → tambah user_id ke context
             │
             ▼
          Handler / Service
             ├─ log kalau perlu
             └─ return sukses / error
        │
        ▼
     (panic ditangkap Recover)
   │
   ▼
AccessLog
   ├─ hitung status
   ├─ tentukan level Info/Warn/Error
   ├─ tulis SATU baris log
   └─ return error ke Echo
   │
   ▼
ErrorHandler
   ├─ ubah error jadi response JSON
   └─ TIDAK logging (biar tidak dobel)
   │
   ▼
Response ke client
```

Poin penting: **`AccessLog` berada di luar `Recover`**. Kalau dibalik, panic
akan melewati `AccessLog` dan request yang panic tidak punya access log.
Sekarang panic tetap tercatat sebagai 500.

---

## 4. Bagaimana `request_id` masuk ke log

### Langkah 1 — middleware menyimpannya ke context

`platform/server/middleware/req_id.go`:

```go
func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := strings.TrimSpace(
				c.Request().Header.Get(echo.HeaderXRequestID),
			)

			if !validRequestID(id) {
				id = uuid.NewV7().String()
			}

			c.Response().Header().Set(echo.HeaderXRequestID, id)

			ctx := logger.WithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}
```

`platform/logger/context.go`:

```go
type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
```

Validasi request ID:

- panjang 1–128 karakter,
- hanya huruf, angka, `-`, `_`, `.`, `:`.

Kalau client mengirim `X-Request-Id` yang aneh (mengandung newline, spasi, atau
karakter lain), ID dibuang dan diganti UUIDv7. Tujuannya:

- mencegah **log injection**,
- menjaga nilai `request_id` tetap aman untuk log pipeline,
- menghindari label cardinality yang tidak terkontrol.

### Langkah 2 — handler menyuntikkannya saat log ditulis

`platform/logger/handler.go`:

```go
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" && !hasAttr(r, KeyRequestID) {
		r.AddAttrs(slog.String(KeyRequestID, id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}
```

Ini kuncinya:

- `slog.InfoContext(ctx, ...)` dan `slog.LogAttrs(ctx, ...)` meneruskan `ctx`
  ke `Handler.Handle`.
- Di `Handle`, `request_id` dibaca dari context.
- `r.AddAttrs(...)` menambahkannya ke record log.
- `hasAttr` mencegah `request_id` dobel kalau sudah ada.

`WithAttrs` dan `WithGroup` harus di-wrap ulang karena `slog.With(...)`
memanggil `Handler.WithAttrs(...)`. Kalau tidak di-wrap, logger turunan akan
kehilangan `contextHandler`, dan `request_id` hilang tanpa error.

### Kenapa `slog.InfoContext` bisa, `slog.Info` tidak?

```go
slog.InfoContext(ctx, "hello") // request_id ikut
slog.Info("hello")             // request_id TIDAK ikut, karena ctx tidak ada
```

Aturan praktis: **di jalur request, selalu pakai varian yang membawa `ctx`**.

---

## 5. Bagaimana `user_id` masuk

Di `platform/server/middleware/requireAuth.go`, setelah token valid:

```go
ctx = context.WithValue(ctx, identityKey{}, id)

ctx = logger.WithContext(
	ctx,
	logger.FromContext(ctx).With("user_id", id.UID),
)

c.SetRequest(c.Request().WithContext(ctx))
```

Yang terjadi:

- `logger.FromContext(ctx)` mengambil logger dari context, atau `slog.Default()`
  kalau belum ada.
- `.With("user_id", id.UID)` membuat logger turunan dengan attribute `user_id`.
- `logger.WithContext(ctx, ...)` menyimpan logger turunan itu ke context.

Lalu `AccessLog` membaca context **setelah** `next(c)` selesai:

```go
ctx := c.Request().Context()
logger.FromContext(ctx).LogAttrs(ctx, level, "http request", attrs...)
```

Karena `RequireAuth` sudah mengganti request context, `AccessLog` melihat logger
yang punya `user_id`. Jadi satu baris access log bisa berisi:

```json
{
  "request_id": "...",
  "user_id": "...",
  "http_method": "GET",
  "http_status": 200
}
```

Catatan penting:

- `slog.InfoContext(ctx, ...)` di handler **hanya** mendapat `request_id`, bukan
  `user_id`, karena `user_id` adalah attribute pada logger turunan.
- Kalau handler ingin ikut `user_id`, pakai `logger.FromContext(ctx)`.

---

## 6. Isi access log

Di `platform/server/middleware/logger.go`:

```go
attrs := []slog.Attr{
	slog.String("http_method", c.Request().Method),
	slog.String("http_route", route),
	slog.String("http_path", c.Request().URL.Path),
	slog.Int("http_status", status),
	slog.Float64("duration_ms",
		float64(time.Since(start).Microseconds())/1000),
	slog.Int64("http_response_bytes", responseBytes),
	slog.String("http_client_ip", c.RealIP()),
	slog.String("http_user_agent",
		truncate(c.Request().UserAgent(), maxUserAgentLen)),
}
```

Arti field-nya:

| Field | Isi |
|---|---|
| `http_method` | `GET`, `POST`, dll. |
| `http_route` | pola route, mis. `/users/:id` |
| `http_path` | path aktual, mis. `/users/42` |
| `http_status` | status yang akan dikirim |
| `duration_ms` | durasi handler |
| `http_response_bytes` | ukuran response |
| `http_client_ip` | IP client |
| `http_user_agent` | User-Agent, dipotong 256 byte |
| `request_id` | otomatis dari context |
| `user_id` | otomatis kalau lewat `RequireAuth` |
| `error` | pesan error |
| `error_kind` | `internal`, `bad_request`, `not_found`, dst. |
| `error_cause` | penyebab internal |
| `stack` | stack trace kalau panic |

Level otomatis:

```go
level := slog.LevelInfo
switch {
case status >= 500:
	level = slog.LevelError
case status >= 400:
	level = slog.LevelWarn
}
```

Jadi:

- 2xx/3xx → `INFO`
- 4xx → `WARN`
- 5xx/panic → `ERROR`

Contoh nyata:

```json
{
  "level": "ERROR",
  "msg": "http request",
  "http_method": "GET",
  "http_route": "/identities/internal",
  "http_path": "/identities/internal",
  "http_status": 500,
  "duration_ms": 0.016,
  "error": "oops something went wrong",
  "error_kind": "internal",
  "error_cause": "dial tcp 127.0.0.1:5432: connect: connection refused",
  "request_id": "01a17734-a7dd-76ef-be0e-d535ca54ba7f"
}
```

Perhatikan:

- `error` adalah pesan yang aman untuk client.
- `error_cause` adalah detail internal.
- Keduanya dicatat di server, tapi response ke client hanya berisi `error`.

Satu catatan teknis: `http_response_bytes` akan `0` untuk response error, karena
`AccessLog` menulis log **sebelum** `ErrorHandler` menulis response JSON. Ini
konsekuensi urutan middleware. Kalau nanti butuh ukuran response error yang
akurat, perlu pendekatan lain (misalnya log setelah response selesai ditulis).

---

## 7. Alur error: kenapa tidak dobel

Aturannya: **log atau return, jangan dua-duanya.**

Yang **salah**:

```go
if err != nil {
	log.Error("db error", "error", err) // log di sini
	return err                          // nanti AccessLog log lagi
}
```

Yang **benar** di service/repository:

```go
if err := s.db.Ping(ctx); err != nil {
	return "", apperr.Wrap(
		fmt.Errorf("ping postgres: %w", err),
		apperr.KindInternal,
		"oops something went wrong",
	)
}
```

Lalu:

1. `apperr.Error()` mengembalikan `Message` — pesan yang aman untuk client.
2. `apperr.Unwrap()` mengembalikan `cause` — detail internal.
3. `AccessLog` mencatat:

```go
attrs = append(attrs, slog.String("error", err.Error()))

var appErr *apperr.Error
if errors.As(err, &appErr) {
	attrs = append(attrs, slog.String("error_kind", appErr.Kind.String()))
	if cause := appErr.Unwrap(); cause != nil {
		attrs = append(attrs, slog.String("error_cause", cause.Error()))
	}
}
```

4. `ErrorHandler` hanya mengirim `Message` ke client:

```go
func ErrorHandler() echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		status, payload := renderError(err)
		_ = c.JSON(status, payload)
	}
}
```

`ErrorHandler` sengaja **tidak** logging. Kalau dia logging juga, satu error
akan muncul dua kali: sekali dari `AccessLog`, sekali dari `ErrorHandler`.

Pengecualian: **background job tanpa HTTP request** tidak punya `AccessLog`.
Di situ boundary-nya adalah worker itu sendiri, jadi log di worker:

```go
if err := process(ctx); err != nil {
	slog.ErrorContext(ctx, "email job failed", "job", "email", "error", err)
}
```

Itu bukan dobel, karena tidak ada `AccessLog` yang mencatatnya.

`RequireAuth` juga mengikuti aturan ini:

```go
id, err := v.Verify(ctx, token)
if err != nil {
	return apperr.Wrap(err, apperr.KindUnauthorized, "invalid or expired token")
}
```

Tidak ada `slog.Warn` di situ. `AccessLog` yang mencatat
`error="invalid or expired token"`, `error_kind="unauthorized"`, dan
`error_cause` dari Firebase, dengan level `WARN` karena statusnya 401.

> **Penting soal `error_cause`:** cause ikut dicatat ke log. Jangan memasukkan
> password, token, DSN lengkap, atau data pribadi ke dalam error cause.

---

## 8. Alur panic

`Recover` dari Echo tidak menulis log sendiri. Dia mengubah panic menjadi
`*PanicStackError`, lalu mengembalikannya sebagai error.

`AccessLog` mendeteksinya:

```go
var panicErr *echomw.PanicStackError
if errors.As(err, &panicErr) {
	attrs = append(attrs,
		slog.String("error", panicErr.Err.Error()),
		slog.String("stack", string(panicErr.Stack)),
	)
}
```

Hasilnya:

- satu baris access log,
- `http_status=500`,
- `level=ERROR`,
- `error` = pesan panic,
- `stack` = stack trace.

Kenapa harus satu baris? Supaya mudah dicari di Loki dengan `request_id`, dan
tidak ada log panic yang terpisah dari konteks request.

Konfigurasi Recover:

```go
emw.RecoverConfig{
	StackSize:       16 << 10, // 16 KB
	DisableStackAll: true,     // hanya goroutine saat ini
}
```

`DisableStackAll: true` mencegah stack semua goroutine ikut dicetak, yang bisa
membuat log sangat besar.

---

## 9. Cara pakai sehari-hari

### Di handler HTTP

```go
func (h *Handler) Foo(c *echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	log.InfoContext(ctx, "fetching foo")

	res, err := h.service.Foo(ctx)
	if err != nil {
		return err // biarkan AccessLog yang mencatat
	}

	return c.JSON(http.StatusOK, map[string]any{"foo": res})
}
```

### Di service

```go
func (s *Service) Foo(ctx context.Context) (string, error) {
	log := logger.FromContext(ctx)
	log.DebugContext(ctx, "checking dependencies")

	if err := s.db.Ping(ctx); err != nil {
		return "", apperr.Wrap(
			fmt.Errorf("ping postgres: %w", err),
			apperr.KindInternal,
			"oops something went wrong",
		)
	}

	return "Bar", nil
}
```

Kalau service hanya meneruskan error, **jangan log**. Kalau service menangani
error dan tidak mengembalikannya (misalnya fallback berhasil), baru log:

```go
if err := cache.Set(ctx, key, value); err != nil {
	log.WarnContext(ctx, "cache set failed, continuing without cache", "error", err)
}
```

### Di background job

```go
func (w *Worker) Run(ctx context.Context) {
	slog.InfoContext(ctx, "worker started", "worker", "email")

	if err := w.process(ctx); err != nil {
		slog.ErrorContext(ctx, "worker failed", "worker", "email", "error", err)
	}
}
```

### Menambah field request-scoped

Misalnya `tenant_id`:

```go
ctx = logger.WithContext(
	ctx,
	logger.FromContext(ctx).With("tenant_id", tenantID),
)
```

Setelah itu, semua log yang memakai `logger.FromContext(ctx)` akan membawa
`tenant_id`.

### Level log

| Level | Kapan dipakai |
|---|---|
| `Debug` | detail development, tidak perlu di production |
| `Info` | lifecycle & operasi normal (startup, request 2xx/3xx) |
| `Warn` | request 4xx, kondisi tak terduga yang masih ditangani |
| `Error` | 5xx, panic, kegagalan yang butuh perhatian |

### Jangan lakukan ini

```go
// ❌ logger baru, bypass handler & base attrs
log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// ❌ tidak bawa context
slog.Info("user created")

// ❌ string interpolation
slog.Info(fmt.Sprintf("user %s created", id))

// ❌ log lalu return error
log.Error("failed", "error", err)
return err

// ❌ log secret/PII
log.Info("login", "password", password)
log.Info("request", "url", r.URL.String()) // query bisa berisi token

// ❌ set request_id manual lewat logger.With
log.With("request_id", id) // bisa dobel; pakai context
```

Yang terakhir: `request_id` seharusnya hanya lewat
`logger.WithRequestID(ctx, id)`. `hasAttr` hanya memeriksa attribute di record,
bukan base attribute logger, jadi menambahkan `request_id` lewat `.With()` bisa
menghasilkan dua field.

### Ringkasan pemilihan API

| Kebutuhan | Pakai |
|---|---|
| Log di jalur request | `logger.FromContext(ctx).InfoContext(ctx, ...)` |
| Log tanpa butuh `user_id` | `slog.InfoContext(ctx, ...)` |
| Log background job | `slog.InfoContext(ctx, ...)` / `slog.ErrorContext(...)` |
| Tambah field per request | `logger.WithContext(ctx, logger.FromContext(ctx).With(...))` |
| Log startup/lifecycle | logger hasil `logger.New(cfg)` atau `slog.Info` |

---

## 10. Konfigurasi environment

| Env | Perilaku |
|---|---|
| `APP_ENV=development` | format text (default), `source` aktif, level default `debug` |
| `APP_ENV=production` | format **selalu JSON**, `source` mati, level default `info` |
| `APP_LOG_LEVEL=debug\|info\|warn\|error` | level minimum |
| `APP_LOG_LEVEL=` kosong | auto: dev `debug`, lainnya `info` |
| `APP_LOG_FORMAT=text\|json` | hanya berpengaruh di development; di luar dev selalu JSON |
| `APP_NAME` | menjadi field `service` |

Nilai `APP_LOG_LEVEL` atau `APP_LOG_FORMAT` yang tidak valid membuat startup
gagal. Ini sengaja: lebih baik gagal cepat daripada typo seperti
`APP_LOG_LEVEL=debig` diam-diam jatuh ke level lain.

Output selalu ke **stdout**, satu objek JSON per baris di production. Ini yang
nanti dibaca Alloy.

---

## 11. Kesiapan Grafana Alloy/Loki

Aplikasi **tidak** push ke Loki. Alurnya:

```text
App → stdout (JSON 1 baris) → Alloy (collect) → Loki → Grafana
```

Yang perlu dijaga di sisi log:

- jadikan label hanya field low-cardinality: `service`, `env`, `level`,
  opsional `http_route`;
- **jangan** jadikan label: `request_id`, `user_id`, `http_path`,
  `http_client_ip`;
- field tersebut taruh sebagai structured metadata / field JSON.

Kalau nanti pakai OpenTelemetry, `trace_id` dan `span_id` cukup ditambahkan di
`platform/logger/handler.go`, di dalam `Handle`. Semua call site yang sudah ada
otomatis ikut, tanpa perlu diubah.

Contoh potongan `loki.process` (sesuaikan source-nya dengan deployment):

```alloy
loki.process "app" {
  forward_to = [loki.write.default.receiver]

  stage.json {
    expressions = {
      level      = "level",
      service    = "service",
      env        = "env",
      request_id = "request_id",
      http_route = "http_route",
    }
  }

  stage.labels {
    values = {
      service = "",
      env     = "",
      level   = "",
    }
  }

  // request_id/http_route tidak dijadikan label; simpan sebagai metadata.
  stage.structured_metadata {
    values = {
      request_id = "",
      http_route = "",
    }
  }
}
```

Contoh query yang akan mudah dilakukan:

```logql
{service="app", env="production"} | json | http_status >= 500
{service="app"} | json | request_id="01a17734-a7dd-76ef-be0e-d535ca54ba7f"
```

---

## 12. Cara memverifikasi sendiri

Test yang ada:

```bash
go test ./platform/logger ./platform/server/middleware -v
```

Test tersebut memeriksa:

- `request_id` dari header masuk ke log,
- request ID berbahaya diganti,
- panic menghasilkan satu log + stack + status 500,
- `apperr` menghasilkan `error`, `error_kind`, `error_cause`,
- format JSON dan base attribute.

Uji manual:

```bash
APP_ENV=production \
APP_LOG_FORMAT=json \
APP_LOG_LEVEL=info \
APP_PORT=18080 \
go run ./cmd/app
```

Lalu di terminal lain:

```bash
curl -i -H 'X-Request-Id: demo-1' localhost:18080/identities/foo
```

Yang harus terlihat:

- response header `X-Request-Id: demo-1`,
- satu baris log `msg="http request"`,
- `request_id="demo-1"`,
- `http_status=200`.

Coba juga:

```bash
curl -i -H 'X-Request-Id: bad id!' localhost:18080/identities/foo
```

ID yang mengandung spasi akan ditolak dan diganti UUIDv7, baik di response
header maupun di log.

Cek duplicate key JSON (opsional, butuh Python):

```bash
python3 - /tmp/app.log <<'PY'
import json
import sys

dups = []
for i, line in enumerate(open(sys.argv[1]), 1):
    line = line.strip()
    if not line:
        continue

    def hook(pairs):
        seen = set()
        for k, _ in pairs:
            if k in seen:
                dups.append((i, k))
            seen.add(k)
        return dict(pairs)

    json.loads(line, object_pairs_hook=hook)

print("duplicate keys:", dups or "none")
PY
```

Script di atas membaca file log dari argumen pertama (`/tmp/app.log`), bukan
dari stdin, supaya bisa dipakai bersama heredoc.

---

## 13. Kalau mau mengubah sesuatu

| Kebutuhan | Sentuh file |
|---|---|
| ganti format/tujuan log | `platform/logger/logger.go` → `NewWithWriter` |
| tambah field otomatis dari context (`trace_id`) | `platform/logger/handler.go` |
| tambah helper context | `platform/logger/context.go` |
| ubah cara request ID dibuat/divalidasi | `platform/server/middleware/req_id.go` |
| tambah field access log | `platform/server/middleware/logger.go` |
| ubah urutan middleware | `platform/server/server.go` |
| tambah field setelah auth | `platform/server/middleware/requireAuth.go` |
| ubah mapping error → HTTP | `platform/server/middleware/error.go` + `apperr` |
| ubah level/format dari env | `platform/config/config.go` + `logger.go` |

---

## 14. Ringkasan mental model

```text
slog.SetDefault(handler + base attrs)
        │
        ├─ semua package log lewat slog / logger.FromContext
        │
        ├─ request masuk
        │    ├─ RequestID  → ctx.request_id
        │    ├─ AccessLog  → mulai timer, panggil next
        │    ├─ Recover    → tangkap panic
        │    ├─ RequireAuth→ ctx.logger + user_id
        │    └─ handler/service
        │
        ├─ setiap log dengan ctx:
        │    contextHandler.Handle(ctx, record)
        │         └─ tambah request_id
        │
        ├─ AccessLog selesai:
        │    ├─ hitung status & level
        │    ├─ tambah error/cause/stack
        │    └─ tulis SATU baris
        │
        └─ ErrorHandler:
             └─ tulis response JSON, tanpa log
```

Intinya:

1. **Satu handler** dipasang sekali lewat `logger.New`.
2. **`request_id` disuntik otomatis** oleh `contextHandler` dari context.
3. **`user_id`** ditambahkan oleh `RequireAuth` sebagai logger turunan.
4. **`AccessLog` adalah satu-satunya tempat error request dicatat**.
5. **`ErrorHandler` hanya menulis response**, supaya tidak ada log dobel.
6. **Panic** ditangkap `Recover`, lalu dicatat `AccessLog` lengkap dengan stack.
7. **JSON ke stdout** membuat log siap dikumpulkan Alloy tanpa aplikasi perlu
   tahu apa-apa tentang Loki.

Dengan pola ini, mau log dari handler, service, middleware, atau worker,
semuanya konsisten dan bisa dikorelasikan tanpa menambahkan `request_id` manual
di setiap call site.
