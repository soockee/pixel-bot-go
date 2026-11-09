package recorder

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBotLogWritesWhileARecordingRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rec")
	bot := NewBotLog(slog.NewTextHandler(io.Discard, nil)) // Info and up, as the app logs
	logger := slog.New(bot)
	logger.Info("before the recording")
	start := time.Now().Add(-1500 * time.Millisecond)
	bot.Start(dir, start)
	logger.Info("before its folder exists") // the recording has not created dir yet
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	logger.With("run", 1).Info("bite detected", "sound_after_splash_ms", 210)
	logger.Debug("search detail")
	bot.Stop()
	logger.Info("after the recording")

	f, err := os.Open(filepath.Join(dir, BotLogFile))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []map[string]any
	for scan := bufio.NewScanner(f); scan.Scan(); {
		var row map[string]any
		if err := json.Unmarshal(scan.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if len(rows) != 1 || rows[0]["msg"] != "bite detected" || rows[0]["sound_after_splash_ms"] != float64(210) {
		t.Fatalf("rows = %v, want the one bite while recording", rows)
	}
	if ms := rows[0]["ms"].(float64); ms < 1500 || ms > 3000 {
		t.Errorf("ms = %v, want about 1500 on the recording's clock", ms)
	}
}
