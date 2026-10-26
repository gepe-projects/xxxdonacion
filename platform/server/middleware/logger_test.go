package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"

	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/config"
	"xxxdonacion/platform/firebase"
	"xxxdonacion/platform/logger"
)

func newAccessLogServer(t *testing.T) (*echo.Echo, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	log := logger.NewWithWriter(&config.App{
		Name:        "test",
		Environment: "production",
		LogLevel:    "debug",
		LogFormat:   "json",
	}, &buf)
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	})

	e := echo.NewWithConfig(echo.Config{Logger: log})
	e.HTTPErrorHandler = ErrorHandler()
	e.Use(RequestID())
	e.Use(AccessLog())
	e.Use(echomw.RecoverWithConfig(echomw.RecoverConfig{
		StackSize:       4 << 10,
		DisableStackAll: true,
	}))

	return e, &buf
}

func request(t *testing.T, e *echo.Echo, method, target string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, values := range header {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func parseLogs(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		out = append(out, entry)
	}
	return out
}

func TestAccessLogCorrelatesRequestID(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/users/:id", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"id": c.Param("id")})
	})

	rec := request(t, e, http.MethodGet, "/users/42", http.Header{
		echo.HeaderXRequestID: {"client-request-1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	entries := parseLogs(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %s", len(entries), buf.String())
	}
	got := entries[0]
	want := map[string]any{
		"msg":         "http request",
		"level":       "INFO",
		"service":     "test",
		"request_id":  "client-request-1",
		"http_method": "GET",
		"http_route":  "/users/:id",
		"http_path":   "/users/42",
		"http_status": float64(200),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v (log: %s)", k, got[k], v, buf.String())
		}
	}
	if got["http_response_bytes"] == float64(0) {
		t.Errorf("http_response_bytes = 0, want > 0 (log: %s)", buf.String())
	}
}

func TestAccessLogRejectsUnsafeRequestID(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/", func(c *echo.Context) error { return c.NoContent(http.StatusNoContent) })

	rec := request(t, e, http.MethodGet, "/", http.Header{
		echo.HeaderXRequestID: {"bad id!"},
	})

	entries := parseLogs(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1", len(entries))
	}
	id, _ := entries[0]["request_id"].(string)
	if id == "" || id == "bad id!" {
		t.Fatalf("request_id = %q, want a generated ID", id)
	}
	if got := rec.Header().Get(echo.HeaderXRequestID); got != id {
		t.Fatalf("response request ID = %q, want %q", got, id)
	}
}

func TestAccessLogLogsPanicOnceWithStack(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/panic", func(c *echo.Context) error { panic("kaboom") })

	rec := request(t, e, http.MethodGet, "/panic", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	entries := parseLogs(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %s", len(entries), buf.String())
	}
	got := entries[0]
	if got["level"] != "ERROR" || got["http_status"] != float64(500) {
		t.Fatalf("level/status = %v/%v, want ERROR/500", got["level"], got["http_status"])
	}
	if got["error"] != "kaboom" {
		t.Errorf("error = %v, want kaboom", got["error"])
	}
	if stack, _ := got["stack"].(string); stack == "" {
		t.Error("stack is empty")
	}
}

func TestAccessLogLogsAppErrorCauseOnce(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/fail", func(c *echo.Context) error {
		return apperr.Wrap(errors.New("db down"), apperr.KindInternal, "oops")
	})

	rec := request(t, e, http.MethodGet, "/fail", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	entries := parseLogs(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %s", len(entries), buf.String())
	}
	got := entries[0]
	if got["error"] != "oops" || got["error_kind"] != "internal" || got["error_cause"] != "db down" {
		t.Fatalf("error fields = %v/%v/%v, want oops/internal/db down",
			got["error"], got["error_kind"], got["error_cause"])
	}
}

type staticVerifier struct {
	identity *firebase.Identity
}

func (v staticVerifier) Verify(context.Context, string) (*firebase.Identity, error) {
	return v.identity, nil
}

func (v staticVerifier) VerifyRevoked(context.Context, string) (*firebase.Identity, error) {
	return v.identity, nil
}

func TestAuthUIDInjectedAfterRequireAuth(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/me", func(c *echo.Context) error {
		slog.InfoContext(c.Request().Context(), "handler log")
		return c.NoContent(http.StatusNoContent)
	}, RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}))

	rec := request(t, e, http.MethodGet, "/me", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	entries := parseLogs(t, buf)
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want 2: %s", len(entries), buf.String())
	}
	for _, got := range entries {
		if got["auth_uid"] != "uid-1" {
			t.Errorf("auth_uid = %v, want uid-1 (log: %s)", got["auth_uid"], buf.String())
		}
		if got["request_id"] == "" {
			t.Errorf("request_id is empty (log: %s)", buf.String())
		}
	}
}

func TestAuthUIDAbsentWhenAuthFails(t *testing.T) {
	e, buf := newAccessLogServer(t)
	e.GET("/me", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}))

	rec := request(t, e, http.MethodGet, "/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	entries := parseLogs(t, buf)
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %s", len(entries), buf.String())
	}
	if _, ok := entries[0]["auth_uid"]; ok {
		t.Fatalf("auth_uid should be absent on failed auth: %s", buf.String())
	}
}
