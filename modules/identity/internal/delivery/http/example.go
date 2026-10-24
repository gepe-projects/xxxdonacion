package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func (h *Handler) Foo(c *echo.Context) error {
	res, err := h.service.Foo(c.Request().Context())
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"foo": res,
	})
}

func (h *Handler) InternalError(c *echo.Context) error {
	return h.service.InternalError(c.Request().Context())
}
