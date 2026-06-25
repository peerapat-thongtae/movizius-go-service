// Package logger provides a small structured logger built on log/slog.
package logger

import (
	"log/slog"
	"os"
)

// New returns a structured JSON logger writing to stderr.
func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
