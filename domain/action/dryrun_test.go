package action

import (
	"bytes"
	"encoding/json"
	"image"
	"log/slog"
	"strings"
	"testing"
)

func TestDryRunLogsInsteadOfSending(t *testing.T) {
	var buf bytes.Buffer
	area := image.Rect(0, 0, 10, 10)
	inFront := true
	var moved []image.Point
	d := DryRun{
		Logger:  slog.New(slog.NewJSONHandler(&buf, nil)),
		InFront: func() bool { return inFront },
		Area:    func() *image.Rectangle { return &area },
		Moved:   func(p image.Point, _ image.Rectangle) { moved = append(moved, p) },
	}
	if d.Tap(0x72, 0) != nil || d.MoveTo(image.Pt(3, 4)) != nil || d.ClickRight() != nil {
		t.Fatal("dry run refused input in front of the game")
	}
	if d.MoveTo(image.Pt(30, 4)) == nil {
		t.Fatal("dry run moved outside the capture area")
	}
	inFront = false
	if d.Tap(0x72, 0) == nil || d.ClickRight() == nil {
		t.Fatal("dry run sent input with the game behind")
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 || len(moved) != 1 {
		t.Fatalf("got %d log lines and %d moves, want 3 and 1: %q", len(lines), len(moved), buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["action"] != "move_cursor" || rec["x"] != float64(3) || rec["y"] != float64(4) {
		t.Fatalf("unexpected record: %v", rec)
	}
}
