package middleware

import (
	"errors"
	"github.com/labstack/echo/v5"
	"net/http"
	"xxxdonacion/platform/apperr"
)

type errorResponse struct {
	Message string `json:"message"`
	Errors  any    `json:"errors,omitempty"`
}

// ErrorHandler maps errors to a consistent JSON response. It deliberately does
// not log: AccessLog already sees the same error with the request metadata, so
// logging here would duplicate every entry.
func ErrorHandler() echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		if res, _ := echo.UnwrapResponse(c.Response()); res != nil && res.Committed {
			return
		}
		status, payload := renderError(err)
		_ = c.JSON(status, payload)
	}
}

func renderError(err error) (int, errorResponse) {
	if appErr, ok := errors.AsType[*apperr.Error](err); ok {
		payload := errorResponse{
			Message: appErr.Message,
		}

		if appErr.ValidationErrors != nil {
			payload.Errors = appErr.ValidationErrors
		}

		return statusFor(appErr.Kind), payload
	}

	status := echo.StatusCode(err)

	if status == 0 {
		status = http.StatusInternalServerError
	}

	message := http.StatusText(status)
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) && httpErr.Message != "" {
		message = httpErr.Message
	}

	return status, errorResponse{
		Message: message,
		Errors:  nil,
	}
}

func statusFor(kind apperr.Kind) int {
	switch kind {
	case apperr.KindBadRequest:
		return http.StatusBadRequest
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
