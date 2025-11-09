package main

import (
	"log/slog"
	"os"
)

// logFilePath is the target JSON log file relative to the working directory.
const logFilePath = "pixel_bot_logs.json"

// NewLogger returns a structured slog.Logger writing JSON entries to a file.
// If the log file can't be opened, it falls back to stdout.
// Multiple calls will each create a handler; prefer a single shared logger.
func NewLogger(level slog.Leveler) *slog.Logger {
	// Truncate existing file on each start (O_TRUNC) to reset logs.
	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	var handler *slog.JSONHandler
	if err != nil {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	}
	return slog.New(handler)
}
