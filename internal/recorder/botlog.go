package recorder

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/internal/dataset"
)

// BotLog is a slog.Handler that passes every record on and, while a recording
// runs, also writes it to the recording's bot.jsonl, so a recording of a bot
// run shows why it cast and reeled, not only that it did.
type BotLog struct {
	next slog.Handler
	sink *botSink
}

type botSink struct {
	mu    sync.Mutex
	dir   string
	start time.Time
	file  *os.File
}

// NewBotLog wraps next.
func NewBotLog(next slog.Handler) *BotLog { return &BotLog{next: next, sink: &botSink{}} }

// Start writes records to dir/bot.jsonl from now on, timed from start. The
// file opens with the first record once dir exists, since the recording
// creates dir itself.
func (b *BotLog) Start(dir string, start time.Time) {
	s := b.sink
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	s.dir, s.start = dir, start
}

// Stop ends writing and closes the file.
func (b *BotLog) Stop() {
	s := b.sink
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	s.dir = ""
}

func (s *botSink) closeLocked() {
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}

// Enabled implements slog.Handler.
func (b *BotLog) Enabled(ctx context.Context, level slog.Level) bool {
	return b.next.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (b *BotLog) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo {
		b.sink.write(r)
	}
	return b.next.Handle(ctx, r)
}

// WithAttrs implements slog.Handler; bot.jsonl gets each record's own attributes.
func (b *BotLog) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BotLog{next: b.next.WithAttrs(attrs), sink: b.sink}
}

// WithGroup implements slog.Handler.
func (b *BotLog) WithGroup(name string) slog.Handler {
	return &BotLog{next: b.next.WithGroup(name), sink: b.sink}
}

func (s *botSink) write(r slog.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == "" {
		return
	}
	if s.file == nil {
		if _, err := os.Stat(s.dir); err != nil {
			return // the recording has not created its folder yet
		}
		f, err := os.OpenFile(filepath.Join(s.dir, dataset.BotLogFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			s.dir = "" // give up for this recording rather than retry on every record
			return
		}
		s.file = f
	}
	row := map[string]any{"ms": r.Time.Sub(s.start).Milliseconds(), "level": r.Level.String(), "msg": r.Message}
	r.Attrs(func(a slog.Attr) bool {
		row[a.Key] = a.Value.Resolve().Any()
		return true
	})
	line, err := json.Marshal(row)
	if err != nil {
		return
	}
	_, _ = s.file.Write(append(line, '\n'))
}
