package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"xxxdonacion/platform/config"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		env   string
		want  slog.Level
	}{
		{"explicit debug", "debug", "production", slog.LevelDebug},
		{"explicit info", "INFO", "development", slog.LevelInfo},
		{"warn alias", "warning", "production", slog.LevelWarn},
		{"error", " error ", "production", slog.LevelError},
		{"auto development", "", "development", slog.LevelDebug},
		{"auto production", "", "production", slog.LevelInfo},
		{"unknown falls back", "verbose", "production", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLevel(tt.level, tt.env); got != tt.want {
				t.Fatalf("parseLevel(%q, %q) = %v, want %v", tt.level, tt.env, got, tt.want)
			}
		})
	}
}

func TestFormatForcesJSONOutsideDevelopment(t *testing.T) {
	if got := format("text", "production"); got != "json" {
		t.Fatalf("format in production = %q, want json", got)
	}
	if got := format("text", "development"); got != "text" {
		t.Fatalf("format in development = %q, want text", got)
	}
	if got := format("", "development"); got != "text" {
		t.Fatalf("empty format in development = %q, want text", got)
	}
}

func TestNewJSONIncludesBaseAttrsAndRequestID(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&config.App{
		Name:        "identity",
		Environment: "production",
		LogLevel:    "debug",
		LogFormat:   "text", // forced to JSON outside development
	}, &buf)
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	})

	ctx := WithRequestID(context.Background(), "req-123")
	l.InfoContext(ctx, "hello", "answer", 42)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log is not JSON: %v\n%s", err, buf.String())
	}
	want := map[string]any{
		"service":         "identity",
		"env":             "production",
		"service_version": "devel",
		"level":           "INFO",
		"msg":             "hello",
		"request_id":      "req-123",
		"answer":          float64(42),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v (log: %s)", k, got[k], v, buf.String())
		}
	}
}

func TestFromContextFallsBackAndKeepsDerivedAttrs(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	})
	slog.SetDefault(base)

	if got := FromContext(context.Background()); got != base {
		t.Fatal("FromContext without a logger should return slog.Default")
	}

	ctx := WithRequestID(context.Background(), "req-9")
	ctx = WithContext(ctx, FromContext(ctx).With("user_id", "u-1"))
	FromContext(ctx).InfoContext(ctx, "scoped")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log is not JSON: %v\n%s", err, buf.String())
	}
	if got["user_id"] != "u-1" {
		t.Errorf("user_id = %v, want u-1", got["user_id"])
	}
}
