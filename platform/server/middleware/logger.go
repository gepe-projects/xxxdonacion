package middleware

import (
	"errors"
	"log/slog"
	"time"

	"github.com/labstack/echo/v5"

	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/logger"
)

func ContextLogger(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		reqId, _ := ctx.Value("request-id").(string)

		log := slog.With(slog.String("request-id", reqId))

		c.SetRequest(c.Request().WithContext(logger.WithContext(ctx, log)))

		return next(c)
	}
}

// AccessLog writes one structured line per request, correlated by request_id.
// Place it outside Recover so a recovered panic is reported as a 500 here.
//
// This is the single place where request errors are logged: the HTTP error
// handler only maps errors to a response.
func AccessLog() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			err := next(c)

			status := statusOf(c, err)
			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}
			ctx := c.Request().Context()
			attrs := []slog.Attr{
				slog.String("method", c.Request().Method),
				slog.String("path", c.Request().URL.Path),
				slog.String("route", c.Path()),
				slog.Int("status", status),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))

				// apperr.Error() only exposes the user-facing message, so the
				// wrapped cause has to be logged explicitly to keep the detail.
				var appErr *apperr.Error
				if errors.As(err, &appErr) {
					attrs = append(attrs, slog.String("kind", appErr.Kind.String()))
					if cause := appErr.Unwrap(); cause != nil {
						attrs = append(attrs, slog.String("cause", cause.Error()))
					}
				}
			}
			logger.FromContext(ctx).LogAttrs(ctx, level, "request", attrs...)
			return err
		}
	}
}

// statusOf reports the status the client will receive. apperr kinds carry no
// HTTP status themselves, so map them here (the response is not yet committed).
func statusOf(c *echo.Context, err error) int {
	if res, _ := echo.UnwrapResponse(c.Response()); res == nil || !res.Committed {
		var appErr *apperr.Error
		if errors.As(err, &appErr) {
			return statusFor(appErr.Kind)
		}
	}
	_, status := echo.ResolveResponseStatus(c.Response(), err)
	return status
}
