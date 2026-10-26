package logger

import (
	"context"
	"log/slog"
)

// contextHandler decorates another handler with attributes that are only
// available from the log call context.
//
// It deliberately only adds request_id today. When OpenTelemetry tracing is
// wired in, trace_id/span_id extraction belongs here too, so every existing
// slog call gets trace correlation without touching call sites.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" && !hasAttr(r, KeyRequestID) {
		r.AddAttrs(slog.String(KeyRequestID, id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs and WithGroup must preserve the wrapper, otherwise derived loggers
// (slog.With) would silently lose context injection.
func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

func hasAttr(r slog.Record, key string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
			return false
		}
		return true
	})
	return found
}
