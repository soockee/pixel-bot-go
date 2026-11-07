package action

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestDryRunLogsInsteadOfInjecting(t *testing.T) {
	var buf bytes.Buffer
	press, move, click := DryRun(slog.New(slog.NewJSONHandler(&buf, nil)))
	press(0x72)
	move(3, 4)
	click()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d log lines, want 3: %q", len(lines), buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["action"] != "move_cursor" || rec["x"] != float64(3) || rec["y"] != float64(4) {
		t.Fatalf("unexpected record: %v", rec)
	}
}
