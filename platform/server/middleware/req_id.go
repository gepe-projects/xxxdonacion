package middleware

import (
	"context"
	"uuid"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"
)

// RequestID reads an inbound X-Request-Id or generates a UUIDv7, echoes it back
// on the response, and stores it in the request context.
func RequestID() echo.MiddlewareFunc {
	return echomw.RequestIDWithConfig(echomw.RequestIDConfig{
		Generator: func() string { return uuid.NewV7().String() },
		RequestIDHandler: func(c *echo.Context, id string) {
			ctx := context.WithValue(c.Request().Context(), "request-id", id)
			c.SetRequest(c.Request().WithContext(ctx))
		},
	})
}
