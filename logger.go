package main

import (
	"log/slog"
	"os"
)

// NewLogger returns a structured slog.Logger with the given level.
func NewLogger(level slog.Leveler) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}
