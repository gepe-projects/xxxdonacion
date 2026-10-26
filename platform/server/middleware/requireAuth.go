package middleware

import (
	"context"
	"strings"

	"github.com/labstack/echo/v5"

	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/firebase"
	"xxxdonacion/platform/logger"
)

type identityKey struct {
}

// RequireAuth hanya memverifikasi signature/expiry/issuer/audience secara lokal.
func RequireAuth(v firebase.Verifier) echo.MiddlewareFunc {
	return requireAuth(v.Verify)
}

// RequireAuthRevoked juga mengecek revocation/disabled ke server Google.
// Dipakai untuk endpoint admin yang sensitif.
func RequireAuthRevoked(v firebase.RevokedVerifier) echo.MiddlewareFunc {
	return requireAuth(v.VerifyRevoked)
}

func requireAuth(verify func(context.Context, string) (*firebase.Identity, error)) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			raw := c.Request().Header.Get(echo.HeaderAuthorization)
			token, ok := strings.CutPrefix(raw, "Bearer ")
			if token = strings.TrimSpace(token); !ok || token == "" {
				return apperr.Unauthorized("missing bearer token")
			}

			ctx := c.Request().Context()
			identity, err := verify(ctx, token)
			if err != nil {
				// Do not log here: AccessLog logs this error once, including the
				// wrapped cause, at warn level with the request metadata.
				return apperr.Wrap(err, apperr.KindUnauthorized, "invalid or expired token")
			}
			if identity == nil {
				return apperr.Unauthorized("invalid or expired token")
			}

			ctx = context.WithValue(ctx, identityKey{}, *identity)
			// auth_uid is injected into every log record by the logger's
			// context handler, so handlers can keep using slog.InfoContext.
			ctx = logger.WithAuthUID(ctx, identity.UID)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// Identity mengambil caller terverifikasi. ok=false kalau RequireAuth belum jalan.
func Identity(ctx context.Context) (firebase.Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(firebase.Identity)
	return identity, ok
}
