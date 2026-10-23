# Panduan Observability (APM & Logging) dengan OpenTelemetry, Echo v5, dan Grafana Cloud

Dokumen ini difokuskan khusus pada **cara implementasi best-practice integrasi OpenTelemetry (OTel)** ke **Grafana Cloud** via **Grafana Alloy**, dengan penekanan pada:
1. Tetap menggunakan **Echo v5** tanpa bergantung pada package resmi `otelecho` (membuat Custom Echo v5 OTel Middleware).
2. Melacak performa end-to-end: dari **kecepatan HTTP endpoint**, logic service, hingga **mendeteksi database query yang lambat** (PostgreSQL + sqlc) dan Redis.
3. Menghubungkan **APM Traces (Tempo)** dengan **Logs (Loki)** secara presisi (*Trace-Log Correlation*).

---

## 1. Solusi Echo v5: Custom OpenTelemetry Middleware

Anda **sangat bisa dan bahkan lebih fleksibel** tetap menggunakan Echo v5 tanpa package `otelecho` resmi. Membuat custom middleware OTel untuk Echo v5 hanya membutuhkan package standar OpenTelemetry:
- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/trace`
- `go.opentelemetry.io/otel/propagation`
- `go.opentelemetry.io/otel/semconv/v1.26.0`

### Mengapa Custom Middleware Lebih Bagus?
- **Zero Incompatibility**: Menghindari lock-in ke middleware v4 yang tidak kompatibel dengan tipe signature `*echo.Context` di Echo v5.
- **Full Control Attributes**: Anda bisa menyaring header, menambahkan custom attribute (misal `tenant_id`, `client_ip`), atau mengontrol status error sesuai selera.
- **Response Header Injection**: Anda bisa langsung menginjeksi `X-Trace-Id` ke HTTP response header sehingga frontend/API client bisa menyalin trace ID tersebut untuk komplain/debugging.

### Blueprint Alur Kerja Middleware Echo v5:
1. **Extract Context**: Ambil W3C TraceContext dari HTTP header yang masuk (`traceparent`) menggunakan `otel.GetTextMapPropagator().Extract()`. Ini memastikan jika ada reverse proxy / gateway di depan, trace-nya tersambung.
2. **Start Server Span**: Buat span baru dengan `trace.WithSpanKind(trace.SpanKindServer)`. Beri nama span berdasarkan route template, misalnya `HTTP GET /identities/foo` (gunakan `c.Path()`, bukan raw URL agar metric tidak meledak/cardinality explosion).
3. **Inject Context ke Request**: Bungkus context tracing baru ke dalam `http.Request` dengan `c.SetRequest(req.WithContext(ctx))`.
4. **Eksekusi Handler Berikutnya**: Jalankan `err := next(c)`.
5. **Record Status & Error**:
   - Jika handler mengembalikan `error`, rekam via `span.RecordError(err)` dan `span.SetStatus(codes.Error, err.Error())`.
   - Catat status code HTTP (`c.Response().Status`) ke atribut span (`http.response.status_code`).
6. **Set Header Response**: Pasang header `c.Response().Header().Set("x-trace-id", span.SpanContext().TraceID().String())`.
7. **End Span**: Panggil `span.End()` di `defer`.

Dengan middleware ini, semua request HTTP yang masuk ke Echo v5 langsung menjadi **Root Span** dari setiap transaksi.

---

## 2. Arsitektur Tracing: Dari Endpoint Hingga Deteksi Query Lambat

Berikut gambaran bagaimana satu HTTP request menghasilkan *waterfall span* di Grafana Cloud Tempo:

```text
Grafana Cloud Tempo (Trace View)
─────────────────────────────────────────────────────────────────────────────────────────────────
[Trace ID: 4bf92f3577b34da6a3ce929d0e0e4736] Total Duration: 350ms
├─ [SERVER] HTTP GET /identities/foo ─────────────────────────────────────────────── (350ms)
│  │  (Di-generate oleh Custom Middleware Echo v5)
│  │
│  ├─ [INTERNAL] identity.service.Foo ────────────────────────────────────────────── (340ms)
│  │  │  (Span manual di service logic jika ingin profil bagian logic tertentu)
│  │  │
│  │  ├─ [CLIENT] redis: get session:123 ────────────────────────────────── (2ms)
│  │  │     (Di-generate otomatis oleh redisotel)
│  │  │
│  │  └─ [CLIENT] db: SELECT * FROM users WHERE active = true ────────────── (330ms) ⚠️ SLOW!
│  │        (Di-generate otomatis oleh otelpgx pada query sqlc)
│  │        Attributes:
│  │          db.system: postgresql
│  │          db.statement: SELECT * FROM users WHERE active = true ...
│  │          db.name: xxxdonacion
```

### Kunci Utama: Context Propagation
Agar *waterfall* di atas terjadi secara otomatis:
1. Custom Middleware Echo memasukkan span context ke dalam `req.Context()`.
2. Handler HTTP mengambil context tersebut: `ctx := c.Request().Context()`.
3. Handler mengoper `ctx` ke Service method: `s.service.Foo(ctx)`.
4. Service mengoper `ctx` ke query sqlc: `s.queries.GetUser(ctx, id)`.
5. Karena `pgxpool` dipasangi `otelpgx`, `otelpgx` membaca `ctx` tersebut dan otomatis membuat **Child Span** di bawah span HTTP/Service!

---

## 3. Komponen Platform Telemetry (`platform/telemetry`)

Buat modul khusus (misal di `platform/telemetry/telemetry.go`) untuk menginisialisasi OpenTelemetry SDK saat aplikasi bootstrap:

### A. Resource Attributes
Beri identitas aplikasi Anda yang akan dibaca oleh Grafana Cloud:
- `service.name`: Nama aplikasi (misal `app` atau `xxxdonacion` dari `APP_NAME`).
- `service.version`: Versi aplikasi (misal `1.0.0`).
- `deployment.environment`: Environment (`development`, `staging`, `production` dari `APP_ENV`).

### B. OTLP Exporter Mengarah ke Alloy
Arahkan exporter ke Grafana Alloy yang sudah berjalan:
- **Tracer Provider**: Gunakan `otlptracegrpc` mengarah ke `localhost:4317` (atau `otlptracehttp` ke `localhost:4318`).
- **Logger Provider**: Gunakan `otlploghttp` atau `otlploggrpc` mengarah ke endpoint Alloy yang sama.
- **Meter Provider (Opsional/Bagus untuk Runtime)**: `otlpmetrichttp` untuk metrik runtime Go (Go GC, goroutines, memory).

### C. Graceful Shutdown Function
Fungsi inisialisasi telemetry wajib mengembalikan fungsi `shutdown func(context.Context) error`.
```go
// Konsep Shutdown Telemetry:
// Memastikan TracerProvider.Shutdown(ctx) dan LoggerProvider.Shutdown(ctx)
// dipanggil saat aplikasi menerima SIGTERM/SIGINT, sehingga data trace terakhir di-flush ke Alloy.
```

---

## 4. Instrumentasi Database (PostgreSQL + pgxpool + sqlc)

Anda tidak perlu menulis kode tracing manual di setiap query sqlc! Cukup pasang tracer di level connection pool.

### Menggunakan `otelpgx`
Paket `github.com/exaring/otelpgx` sudah ada di `go.mod` Anda. Cara pasangnya di `platform/postgres/postgres.go`:

1. Parse konfigurasi pool: `poolConfig, err := pgxpool.ParseConfig(cfg.URL)`
2. Pasang tracer ke `poolConfig.ConnConfig.Tracer`:
   ```go
   poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(
       otelpgx.WithTrimSQLInSpanName(), // Merapikan nama span agar mudah dibaca di Grafana
   )
   ```
3. Buat pool dengan konfigurasi tersebut: `pgxpool.NewWithConfig(ctx, poolConfig)`.

### Bagaimana sqlc Bekerja dengan Ini?
sqlc menghasilkan kode Go seperti:
```go
func (q *Queries) GetUser(ctx context.Context, id int64) (User, error) {
    row := q.db.QueryRow(ctx, getUser, id)
    ...
}
```
Karena parameter pertamanya adalah `ctx`, `pgxpool` yang dipanggil di balik layar sqlc otomatis memicu `otelpgx`. Setiap query database akan otomatis memunculkan span di Grafana Tempo dengan info:
- Waktu mulai dan durasi query (hingga satuan milidetik / mikrodetik).
- Teks query SQL (`db.statement`).
- Database host dan nama database.

---

## 5. Instrumentasi Redis (`redisotel`)

Paket `github.com/redis/go-redis/extra/redisotel/v9` juga sudah ada di `go.mod` Anda.

Di `platform/redis/redis.go`:
1. Buat client Redis: `client := goredis.NewClient(...)`
2. Pasang tracing plugin:
   ```go
   if err := redisotel.InstrumentTracing(client); err != nil {
       // handle error
   }
   ```
3. Setiap kali Anda memanggil `client.Get(ctx, key)` atau operasi redis lainnya dengan `ctx`, span Redis akan otomatis muncul berdampingan dengan span database di Grafana Tempo.

---

## 6. Korelasi Log dan APM (Trace ➔ Logs & Logs ➔ Trace)

Agar log yang masuk ke Grafana Loki terhubung langsung dengan trace di Grafana Tempo:

### Gunakan Bridge `otelslog`
Integrasikan Go `log/slog` dengan OpenTelemetry Logger Provider:
1. Pasang bridge `go.opentelemetry.io/contrib/bridges/otelslog`.
2. Jadikan logger tersebut sebagai default:
   ```go
   logger := otelslog.NewLogger("app")
   slog.SetDefault(logger)
   ```
3. **Aturan Utama saat Logging**: Selalu gunakan varian `Context`:
   - ✅ `slog.InfoContext(ctx, "processing order", "order_id", orderID)`
   - ✅ `slog.ErrorContext(ctx, "database query failed", "error", err)`
   - ❌ `slog.Info("processing order")` (jangan gunakan ini untuk request-scoped log karena tidak membawa trace context).

Ketika log dikirim ke Grafana Loki melalui Alloy, log tersebut otomatis menyertakan atribut `trace_id` dan `span_id`.

---

## 7. Cara Memantau & Menemukan Query/Endpoint Lambat di Grafana Cloud

Setelah aplikasi mengirim OTLP ke Alloy dan diteruskan ke Grafana Cloud, berikut cara menganalisis performanya di UI Grafana Cloud:

### A. Menemukan Endpoint Lambat di Grafana Tempo (Traces)
1. Masuk ke Grafana Cloud ➔ Menu **Explore** ➔ Pilih datasource **Tempo**.
2. Gunakan **TraceQL** untuk mencari request yang lambat, misalnya:
   ```text
   { .http.response.status_code = 200 && duration > 500ms }
   ```
   Query di atas akan langsung menampilkan semua request yang memakan waktu lebih dari 500 milidetik.
3. Klik salah satu trace dari daftar pencarian.
4. Anda akan melihat waterfall span: bar span mana yang paling panjang/lebar secara visual adalah penyebab bottleneck!

### B. Menemukan Query Database yang Lambat (Slow SQL Queries)
1. Di Grafana Tempo, cari span dengan filter database:
   ```text
   { .db.system = "postgresql" && duration > 200ms }
   ```
2. Hasilnya adalah daftar seluruh query PostgreSQL yang dieksekusi lebih dari 200 milidetik.
3. Klik span query tersebut untuk melihat atribut `db.statement` (query SQL lengkap) dan trace induknya untuk mengetahui endpoint mana yang memicu query lambat tersebut.

### C. Menggunakan Tombol "Trace to Logs"
1. Pada tampilan trace yang lambat atau error di Tempo, klik tombol **"Logs for this span"** (atau **"Trace to logs"**).
2. Grafana akan otomatis membuka split-view ke Loki yang memfilter log khusus dengan `trace_id` tersebut.
3. Anda bisa langsung membaca log debug atau error yang dicetak oleh aplikasi pada milidetik yang sama tanpa perlu mencari manual di tumpukan log.

### D. Melihat APM Service Graph & RED Metrics
Grafana Cloud secara otomatis membuat metrik RED (Rate, Errors, Duration) dari span yang dikirim:
- **Throughput**: Request per detik per endpoint.
- **Latency Histogram**: p50, p90, p95, dan p99 response time dari setiap route HTTP.
- **Failure Rate**: Persentase HTTP 5xx atau database connection timeout.

---

## 8. Ringkasan Checklist Penerapan (Untuk Anda Kerjakan)

Berikut alur kerja implementasi praktis yang bisa Anda lakukan:

1. **Buat Custom Echo v5 Middleware**:
   - Tempatkan di `platform/server/middleware.go` atau `platform/telemetry/middleware.go`.
   - Gunakan `otel.GetTextMapPropagator()` dan `tracer.Start()`.
   - Pasang di `platform/server/server.go`: `e.Use(OpenTelemetryMiddleware("xxxdonacion"))`.
2. **Setup Telemetry di Platform**:
   - Inisialisasi TracerProvider dan LoggerProvider di `platform/telemetry`.
   - Exporter diarahkan ke Alloy OTLP gRPC `localhost:4317` atau HTTP `localhost:4318`.
3. **Aktifkan `otelpgx` pada Postgres Pool**:
   - Di `platform/postgres/postgres.go`, assign `poolConfig.ConnConfig.Tracer = otelpgx.NewTracer()`.
4. **Aktifkan `redisotel` pada Redis**:
   - Di `platform/redis/redis.go`, jalankan `redisotel.InstrumentTracing(client)`.
5. **Gunakan Contextual Logging**:
   - Ganti pemanggilan `slog.Error(...)` di handler/service menjadi `slog.ErrorContext(ctx, ...)`.
6. **Hubungkan Shutdown Hook**:
   - Di `cmd/app/bootstrap.go`, panggil fungsi shutdown telemetry saat server HTTP berhenti agar span/log tidak ada yang tertinggal di buffer.
