package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/platform/firebase"
)

func TestRateLimitReturnsTooManyRequests(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler()
	e.GET("/limited", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	},
		RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}),
		RateLimit(rdb, "test", 2, time.Minute, IdentityKey),
	)

	for i := 0; i < 2; i++ {
		rec := request(t, e, http.MethodGet, "/limited", http.Header{
			echo.HeaderAuthorization: {"Bearer token"},
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("request %d: status = %d, want 204", i+1, rec.Code)
		}
	}

	rec := request(t, e, http.MethodGet, "/limited", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is missing")
	}
}

func TestRateLimitUsesUIDNotIP(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler()
	handler := func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}
	e.GET("/one", handler,
		RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}),
		RateLimit(rdb, "test", 1, time.Minute, IdentityKey),
	)
	e.GET("/two", handler,
		RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-2"}}),
		RateLimit(rdb, "test", 1, time.Minute, IdentityKey),
	)

	token := http.Header{echo.HeaderAuthorization: {"Bearer token"}}
	if rec := request(t, e, http.MethodGet, "/one", token); rec.Code != http.StatusNoContent {
		t.Fatalf("user 1 first request: status = %d, want 204", rec.Code)
	}
	if rec := request(t, e, http.MethodGet, "/one", token); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("user 1 second request: status = %d, want 429", rec.Code)
	}
	// IP sama, UID berbeda: user 2 tidak boleh ikut kena limit user 1.
	if rec := request(t, e, http.MethodGet, "/two", token); rec.Code != http.StatusNoContent {
		t.Fatalf("user 2 first request: status = %d, want 204", rec.Code)
	}
}

func TestRateLimitFailsOpenWhenRedisIsDown(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	mr.Close()

	e := echo.New()
	e.HTTPErrorHandler = ErrorHandler()
	e.GET("/limited", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	},
		RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}),
		RateLimit(rdb, "test", 1, time.Minute, IdentityKey),
	)

	rec := request(t, e, http.MethodGet, "/limited", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 when Redis is down", rec.Code)
	}
}
