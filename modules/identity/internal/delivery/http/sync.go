package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/server/middleware"
	"xxxdonacion/platform/server/response"
)

type userResponse struct {
	ID     string        `json:"id"`
	Email  string        `json:"email"`
	Name   string        `json:"name,omitempty"`
	Status domain.Status `json:"status"`
	Roles  []domain.Role `json:"roles"`
}

func newUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:     u.ID.String(),
		Email:  u.Email,
		Name:   u.Name,
		Status: u.Status,
		Roles:  u.Roles,
	}
}

func callerFrom(c *echo.Context) (domain.Principal, error) {
	identity, ok := middleware.Identity(c.Request().Context())
	if !ok {
		return domain.Principal{}, apperr.Unauthorized("unauthenticated")
	}
	return domain.Principal{
		UID:           identity.UID,
		Email:         identity.Email,
		Name:          identity.Name,
		EmailVerified: identity.EmailVerified,
	}, nil
}

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
	identity, ok := middleware.Identity(ctx)
	if !ok {
		return apperr.Unauthorized("unauthenticated")
	}
	if _, err := h.service.Authorize(ctx, identity.UID); err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, map[string]any{
		"uid":            identity.UID,
		"email":          identity.Email,
		"name":           identity.Name,
		"email_verified": identity.EmailVerified,
	})
}
