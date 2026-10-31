package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"xxxdonacion/platform/server/response"
)

func (h *Handler) Sync(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	user, err := h.service.Sync(c.Request().Context(), caller)
	if err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, newUserResponse(user))
}

func (h *Handler) Me(c *echo.Context) error {
	ctx := c.Request().Context()
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}
	if _, err := h.service.Authorize(ctx, caller.UID); err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, meResponse{
		UID:           caller.UID,
		Email:         caller.Email,
		Name:          caller.Name,
		EmailVerified: caller.EmailVerified,
	})
}
