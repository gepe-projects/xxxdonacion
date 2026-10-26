package logger

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// WithContext stores l in ctx so downstream middleware and handlers can log
// with request-scoped attributes (user_id, tenant_id, ...).
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext returns the logger stored in ctx, or slog.Default when there is
// none. It never returns nil, so callers can always chain .Info/.Error.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

type requestIDKey struct{}

// WithRequestID attaches a request ID to ctx. The context handler installed by
// New adds it to every record logged with that context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request ID stored in ctx, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
