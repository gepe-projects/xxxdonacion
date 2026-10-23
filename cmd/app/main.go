package main

import (
	"log/slog"
	"os"
)

func main() {
	if err := run(); err != nil {
		slog.Error("app stopped", "error", err)
		os.Exit(1)
	}
}
