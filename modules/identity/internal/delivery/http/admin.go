package http

import (
	"net/http"
	"uuid"

	"github.com/labstack/echo/v5"

	"xxxdonacion/modules/identity/internal/delivery/http/req"
	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/pagination"
	"xxxdonacion/platform/server/response"
)

func (h *Handler) CreateUser(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	var request req.CreateUser
	if err := c.Bind(&request); err != nil {
		return apperr.Wrap(err, apperr.KindBadRequest, "invalid request body")
	}

	user, err := h.service.CreateUser(c.Request().Context(), caller, request.Input())
	if err != nil {
		return err
	}

	return response.JSON(c, http.StatusCreated, newUserResponse(user))
}

func (h *Handler) SetUserStatus(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return apperr.BadRequest("invalid user id")
	}

	var request req.UpdateStatus
	if err := c.Bind(&request); err != nil {
		return apperr.Wrap(err, apperr.KindBadRequest, "invalid request body")
	}

	user, err := h.service.SetUserStatus(c.Request().Context(), caller, userID, request.Input())
	if err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, newUserResponse(user))
}

func (h *Handler) SetUserRoles(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return apperr.BadRequest("invalid user id")
	}

	var request req.UpdateRoles
	if err := c.Bind(&request); err != nil {
		return apperr.Wrap(err, apperr.KindBadRequest, "invalid request body")
	}

	user, err := h.service.SetUserRoles(c.Request().Context(), caller, userID, request.Input())
	if err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, newUserResponse(user))
}

func (h *Handler) DeleteUser(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return apperr.BadRequest("invalid user id")
	}

	if err := h.service.DeleteUser(c.Request().Context(), caller, userID); err != nil {
		return err
	}

	return response.JSON(c, http.StatusOK, nil)
}

func (h *Handler) ListUsers(c *echo.Context) error {
	caller, err := callerFrom(c)
	if err != nil {
		return err
	}

	limit, err := pagination.ParseLimit(c.QueryParam("limit"))
	if err != nil {
		return apperr.BadRequest("limit must be a positive integer")
	}

	var cursor *uuid.UUID
	if raw := c.QueryParam("cursor"); raw != "" {
		id, err := pagination.DecodeUUID(raw)
		if err != nil {
			return apperr.BadRequest("invalid cursor")
		}
		cursor = &id
	}

	page, err := h.service.ListUsers(c.Request().Context(), caller, limit, cursor)
	if err != nil {
		return err
	}

	res := pagination.Page[userResponse]{
		Items:      make([]userResponse, 0, len(page.Items)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for i := range page.Items {
		res.Items = append(res.Items, newUserResponse(&page.Items[i]))
	}

	return response.JSON(c, http.StatusOK, res)
}
