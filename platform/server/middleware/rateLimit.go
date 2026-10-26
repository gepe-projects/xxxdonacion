package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"xxxdonacion/platform/apperr"
	"xxxdonacion/platform/logger"
)

// RateLimit membatasi request per key memakai Redis, sehingga berlaku lintas
// instance. Kalau Redis bermasalah, request tetap dilayani (fail-open) dan
// error hanya di-log.
func RateLimit(
	rdb *redis.Client,
	scope string,
	limit int64,
	window time.Duration,
	keyFn func(*echo.Context) string,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if rdb == nil || limit <= 0 || window <= 0 {
				return next(c)
			}

			key := scope + ":" + keyFn(c)
			count, err := incrementRateLimit(c.Request().Context(), rdb, key, window)
			if err != nil {
				logger.FromContext(c.Request().Context()).Warn("rate limit failed", "error", err)
				return next(c)
			}
			if count > limit {
				retryAfter := int64(window.Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				c.Response().Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
				return apperr.TooManyRequests("too many requests")
			}

			return next(c)
		}
	}
}

// IdentityKey memakai UID hasil RequireAuth; fallback ke IP kalau belum ada.
func IdentityKey(c *echo.Context) string {
	if identity, ok := Identity(c.Request().Context()); ok && identity.UID != "" {
		return identity.UID
	}
	return c.RealIP()
}

func incrementRateLimit(
	ctx context.Context,
	rdb *redis.Client,
	key string,
	window time.Duration,
) (int64, error) {
	pipe := rdb.TxPipeline()
	pipe.SetNX(ctx, key, 0, window)
	incr := pipe.Incr(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
