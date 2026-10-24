package logger

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// WithContext stores l in ctx so downstream middleware and handlers can log
// with request-scoped attributes.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext returns the logger stored in ctx, or slog.Default when there is
// none.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
