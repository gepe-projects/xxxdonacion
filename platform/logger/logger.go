package logger

import (
	"log/slog"
	"os"
	"strings"
	"xxxdonacion/platform/config"
)

func New(cfg *config.App) {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel, cfg.Environment)}

	var h slog.Handler = slog.NewTextHandler(os.Stdout, opts)
	if format(cfg.LogFormat, cfg.Environment) == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)
}

func parseLevel(level, env string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	if !isDevelopment(env) {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// format forces JSON outside development, regardless of APP_LOG_FORMAT.
func format(raw, env string) string {
	if !isDevelopment(env) {
		return "json"
	}
	switch v := strings.ToLower(strings.TrimSpace(raw)); v {
	case "json", "text":
		return v
	}
	// fallback
	return "json"
}

func isDevelopment(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "development")
}
