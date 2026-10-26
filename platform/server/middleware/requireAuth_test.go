package middleware

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"

	"xxxdonacion/platform/firebase"
)

func TestIdentityFromContextAfterRequireAuth(t *testing.T) {
	e, _ := newAccessLogServer(t)

	var got firebase.Identity
	var ok bool
	e.GET("/me", func(c *echo.Context) error {
		got, ok = Identity(c.Request().Context())
		return c.NoContent(http.StatusNoContent)
	}, RequireAuth(staticVerifier{identity: &firebase.Identity{
		UID:           "uid-1",
		Email:         "user@example.com",
		EmailVerified: true,
		Name:          "User",
	}}))

	rec := request(t, e, http.MethodGet, "/me", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !ok {
		t.Fatal("Identity() did not return an identity")
	}
	if got.UID != "uid-1" || got.Email != "user@example.com" || !got.EmailVerified || got.Name != "User" {
		t.Fatalf("identity = %+v, want the verified identity", got)
	}
}

func TestRequireAuthRevokedSetsIdentity(t *testing.T) {
	e, _ := newAccessLogServer(t)

	e.GET("/admin", func(c *echo.Context) error {
		identity, ok := Identity(c.Request().Context())
		if !ok || identity.UID != "uid-1" {
			t.Fatalf("identity = %+v, ok = %v", identity, ok)
		}
		return c.NoContent(http.StatusNoContent)
	}, RequireAuthRevoked(staticVerifier{identity: &firebase.Identity{UID: "uid-1"}}))

	rec := request(t, e, http.MethodGet, "/admin", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestRequireAuthRejectsNilIdentity(t *testing.T) {
	e, _ := newAccessLogServer(t)
	e.GET("/me", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, RequireAuth(staticVerifier{identity: nil}))

	rec := request(t, e, http.MethodGet, "/me", http.Header{
		echo.HeaderAuthorization: {"Bearer token"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
