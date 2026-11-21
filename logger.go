package main

import (
	"log/slog"
	"os"
	"sync"

	"github.com/soocke/pixel-bot-go/config"
)

// NewLogger returns a structured slog.Logger writing JSON entries to
// config.LogPath. It keeps the logs of the last config.LogsKept starts
// (pixel_bot_logs.1.json is the one before), so a problem is still in the log
// after the app was restarted, and writes the detail lines (Debug) too, which
// say why a bite or a pointer move went wrong. If the file can't be opened, it
// falls back to stdout. Prefer one shared logger.
func NewLogger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	w, err := openLog(config.LogPath)
	if err != nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// logLimit is the size at which a run starts the next log file; the detail
// lines take a few MB an hour, so only a very long run reaches it.
const logLimit = 20 << 20

// logFile is a log that moves on to a fresh file, keeping the older ones, once
// it reaches limit.
type logFile struct {
	mu      sync.Mutex
	path    string
	f       *os.File
	written int64
	limit   int64
}

func openLog(path string) (*logFile, error) {
	l := &logFile{path: path, limit: logLimit}
	return l, l.rotate()
}

// rotate shifts the kept logs by one, dropping the oldest, and opens a new one.
func (l *logFile) rotate() error {
	if l.f != nil {
		l.f.Close()
	}
	for i := config.LogsKept - 1; i > 0; i-- {
		_ = os.Rename(config.KeptLogPath(l.path, i-1), config.KeptLogPath(l.path, i))
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	l.f, l.written = f, 0
	return err
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.written+int64(len(p)) > l.limit {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.written += int64(n)
	return n, err
}
