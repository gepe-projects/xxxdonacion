// Package logger owns the single application logger. Everything in the app
// logs through slog.Default() (with the context handler installed here), so the
// output format, base attributes, and levels stay consistent everywhere.
package logger

import (
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"xxxdonacion/platform/config"
)

// KeyRequestID is the structured field used to correlate all log lines that
// belong to one inbound HTTP request. Keep the name stable: dashboards and log
// pipelines (e.g. Grafana Alloy/Loki) will query it.
const KeyRequestID = "request_id"

// KeyAuthUID is attached to every log record whose context has a verified
// Firebase UID. It is only present on routes that ran RequireAuth successfully.
const KeyAuthUID = "auth_uid"

// New builds the application logger, installs it as slog's default, and
// returns it. Call it once after config is loaded, before anything else logs.
func New(cfg *config.App) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

// NewWithWriter is New with an explicit sink. It exists for tests; production
// code should use New.
func NewWithWriter(cfg *config.App, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: parseLevel(cfg.LogLevel, cfg.Environment),
		// Caller information is helpful locally but costs a runtime.Caller on
		// every record, so keep it out of production.
		AddSource: isDevelopment(cfg.Environment),
	}

	var h slog.Handler
	if format(cfg.LogFormat, cfg.Environment) == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}

	// Inject request-scoped attributes (currently request_id) from the context
	// into every record. This keeps slog.InfoContext/LogAttrs correct even when
	// callers forget to derive a logger with With().
	h = &contextHandler{Handler: h}

	base := []any{
		"service", cfg.Name,
		"env", cfg.Environment,
	}
	if version, commit := buildInfo(); version != "" {
		// Prefix with service_ so we never collide with fields emitted by
		// libraries (Echo logs its own "version" on startup).
		base = append(base, "service_version", version)
		if commit != "" {
			base = append(base, "service_commit", commit)
		}
	}

	log := slog.New(h).With(base...)
	slog.SetDefault(log)
	return log
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
	if isDevelopment(env) {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// format forces JSON outside development, regardless of APP_LOG_FORMAT, so log
// collectors always receive one JSON object per line.
func format(raw, env string) string {
	if !isDevelopment(env) {
		return "json"
	}
	switch v := strings.ToLower(strings.TrimSpace(raw)); v {
	case "json", "text":
		return v
	}
	return "text"
}

func isDevelopment(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "development")
}

// buildInfo returns the module version and a short VCS revision when the
// binary was built with VCS metadata. `go run` usually reports "(devel)".
func buildInfo() (version, commit string) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "devel", ""
	}

	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		version = "devel"
	} else {
		version = info.Main.Version
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			commit = setting.Value
			if len(commit) > 12 {
				commit = commit[:12]
			}
			break
		}
	}
	return version, commit
}
