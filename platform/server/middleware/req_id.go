package middleware

import (
	"strings"
	"uuid"

	"github.com/labstack/echo/v5"

	"xxxdonacion/platform/logger"
)

const maxRequestIDLen = 128

// RequestID reads an inbound X-Request-Id, validates it, or generates a
// UUIDv7, stores it on the request context, and echoes it back on the response.
//
// Inbound IDs are untrusted: accept only a small, log-safe character set so a
// client cannot inject newlines into text logs or blow up label cardinality.
func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := strings.TrimSpace(c.Request().Header.Get(echo.HeaderXRequestID))
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

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}
