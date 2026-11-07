package action

import (
	"image"
	"log/slog"
	"slices"
	"testing"
)

type guardFixture struct {
	Guard
	inFront   bool
	area      *image.Rectangle
	cursor    image.Point
	sent      []string
	takeovers int
}

func newGuardFixture() *guardFixture {
	area := image.Rect(100, 100, 200, 200)
	f := &guardFixture{inFront: true, area: &area}
	f.Guard = Guard{
		Logger:     slog.New(slog.DiscardHandler),
		InFront:    func() bool { return f.inFront },
		Area:       func() *image.Rectangle { return f.area },
		CursorPos:  func() (int, int, bool) { return f.cursor.X, f.cursor.Y, true },
		OnTakeover: func() { f.takeovers++ },
		Press:      func(byte) { f.sent = append(f.sent, "press") },
		Move: func(x, y int) {
			f.cursor = image.Pt(x, y)
			f.sent = append(f.sent, "move")
		},
		Click: func() { f.sent = append(f.sent, "click") },
	}
	return f
}

func TestGuardAllowsNormalCycle(t *testing.T) {
	f := newGuardFixture()
	f.PressKey(1)
	f.MoveCursor(150, 150)
	f.MoveCursor(150, 150)
	f.ClickRight()
	if want := []string{"press", "move", "move", "click"}; !slices.Equal(f.sent, want) {
		t.Errorf("sent = %v, want %v", f.sent, want)
	}
	if f.takeovers != 0 {
		t.Errorf("takeovers = %d, want 0", f.takeovers)
	}
}

func TestGuardDropsInputWhenWindowIsNotInFront(t *testing.T) {
	f := newGuardFixture()
	f.MoveCursor(150, 150)
	f.sent = nil
	f.inFront = false
	f.PressKey(1)
	f.MoveCursor(150, 150)
	f.ClickRight()
	if len(f.sent) != 0 {
		t.Errorf("sent = %v, want nothing", f.sent)
	}
}

func TestGuardKeepsMovesInsideTheArea(t *testing.T) {
	f := newGuardFixture()
	f.MoveCursor(50, 150)
	f.ClickRight()
	f.area = nil
	f.MoveCursor(150, 150)
	if len(f.sent) != 0 {
		t.Errorf("sent = %v, want nothing", f.sent)
	}
}

func TestGuardHandsOverWhenTheUserMovesTheCursor(t *testing.T) {
	tests := []struct {
		name string
		act  func(*guardFixture)
	}{
		{"before the click", func(f *guardFixture) { f.ClickRight() }},
		{"before the next move", func(f *guardFixture) { f.MoveCursor(150, 150) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGuardFixture()
			f.MoveCursor(150, 150)
			f.sent = nil
			f.cursor = image.Pt(151, 150)
			tt.act(f)
			if len(f.sent) != 0 {
				t.Errorf("sent = %v, want nothing", f.sent)
			}
			if f.takeovers != 1 {
				t.Errorf("takeovers = %d, want 1", f.takeovers)
			}
		})
	}
}
