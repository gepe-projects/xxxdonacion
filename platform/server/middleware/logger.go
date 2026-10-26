package middleware

import (
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	echomw "github.com/labstack/echo/v5/middleware"

	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/logger"
)

const maxUserAgentLen = 256

// AccessLog writes one structured line per request. It must be installed
// outside Recover (Recover is the inner middleware) so a recovered panic still
// produces an access log line with status 500 and the stack trace.
//
// This is the single place where request errors are logged: the HTTP error
// handler only maps errors to a response, and libraries return errors instead
// of logging them. That keeps every failure to exactly one log entry.
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

			route := c.Path()
			if route == "" {
				route = "unmatched"
			}

			var responseBytes int64
			if res, _ := echo.UnwrapResponse(c.Response()); res != nil {
				responseBytes = res.Size
			}

			attrs := []slog.Attr{
				slog.String("http_method", c.Request().Method),
				slog.String("http_route", route),
				slog.String("http_path", c.Request().URL.Path),
				slog.Int("http_status", status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
				slog.Int64("http_response_bytes", responseBytes),
				slog.String("http_client_ip", c.RealIP()),
				slog.String("http_user_agent", truncate(c.Request().UserAgent(), maxUserAgentLen)),
			}

			// PanicStackError embeds the whole stack in Error(); keep the stack
			// in its own field so the error message stays readable and queryable.
			var panicErr *echomw.PanicStackError
			if errors.As(err, &panicErr) {
				attrs = append(attrs,
					slog.String("error", panicErr.Err.Error()),
					slog.String("stack", string(panicErr.Stack)),
				)
			} else if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))

				// apperr.Error() only exposes the user-facing message, so the
				// wrapped cause has to be logged explicitly to keep the detail.
				var appErr *apperr.Error
				if errors.As(err, &appErr) {
					attrs = append(attrs, slog.String("error_kind", appErr.Kind.String()))
					if cause := appErr.Unwrap(); cause != nil {
						attrs = append(attrs, slog.String("error_cause", cause.Error()))
					}
				}
			}

			// The context handler adds request_id and auth_uid; FromContext also
			// carries any extra request-scoped logger attributes.
			ctx := c.Request().Context()
			logger.FromContext(ctx).LogAttrs(ctx, level, "http request", attrs...)

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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Do not cut a multi-byte rune in half.
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
