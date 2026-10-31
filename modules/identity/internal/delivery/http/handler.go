package http

import (
	"github.com/labstack/echo/v5"

	"xxxdonacion/modules/identity/api/domain"
	"xxxdonacion/modules/identity/internal/service"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/server/middleware"
)

type Handler struct {
	service *service.Service
}

func New(svc *service.Service) *Handler {
	return &Handler{service: svc}
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
