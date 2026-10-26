package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/modules/identity/internal/service"
	"xxxdonacion/platform/firebase"
	"xxxdonacion/platform/server/middleware"
)

func newMeHandler(t *testing.T, cachedUser string) *echo.Echo {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := mr.Set("identity:user:uid-1", cachedUser); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	h := New(service.New(nil, rdb, nil))
	e.GET("/me", h.Me, middleware.RequireAuth(staticVerifier{identity: &firebase.Identity{
		UID:           "uid-1",
		Email:         "user@example.com",
		Name:          "User",
		EmailVerified: true,
	}}))
	return e
}

type staticVerifier struct {
	identity *firebase.Identity
}

func (v staticVerifier) Verify(context.Context, string) (*firebase.Identity, error) {
	return v.identity, nil
}

func TestMeReturnsIdentityFromContext(t *testing.T) {
	e := newMeHandler(t, `{"status":"ACTIVE","roles":["USER"]}`)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			UID           string `json:"uid"`
			Email         string `json:"email"`
			Name          string `json:"name"`
			EmailVerified bool   `json:"email_verified"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.UID != "uid-1" || body.Data.Email != "user@example.com" ||
		body.Data.Name != "User" || !body.Data.EmailVerified {
		t.Fatalf("data = %+v, want identity from context", body.Data)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = middleware.ErrorHandler()
	h := New(service.New(nil, nil, nil))
	e.GET("/me", h.Me, middleware.RequireAuth(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}))

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMeRejectsDisabledUser(t *testing.T) {
	e := newMeHandler(t, `{"status":"DISABLED","roles":[]}`)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
