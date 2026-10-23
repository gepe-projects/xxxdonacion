package http

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
)

func (h *Handler) Foo(c *echo.Context) error {
	res, err := h.service.Foo(c.Request().Context())
	if err != nil {
		slog.Error("GET /foo", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error": "internal server error",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"foo": res,
	})
}
