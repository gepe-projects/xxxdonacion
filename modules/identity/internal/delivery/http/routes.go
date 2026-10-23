package http

import (
	"xxxdonacion/modules/identity/api/domain"
)

type Handler struct {
	service domain.Service
}

func New(service domain.Service) *Handler {
	return &Handler{service}
}
