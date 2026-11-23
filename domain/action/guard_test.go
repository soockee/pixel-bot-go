package action

import (
	"errors"
	"image"
	"log/slog"
	"slices"
	"testing"
	"time"
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
		Press:      func(byte, time.Duration) error { f.sent = append(f.sent, "press"); return nil },
		Move: func(x, y int) error {
			f.cursor = image.Pt(x, y)
			f.sent = append(f.sent, "move")
			return nil
		},
		Click:   func() error { f.sent = append(f.sent, "click"); return nil },
		CanDrag: func() bool { return true },
		Drag:    func(int) error { f.sent = append(f.sent, "drag"); return nil },
	}
	return f
}

func TestGuardAllowsNormalCycle(t *testing.T) {
	f := newGuardFixture()
	f.Tap(1, 0)
	f.MoveTo(image.Pt(150, 150))
	f.MoveTo(image.Pt(150, 150))
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
	f.MoveTo(image.Pt(150, 150))
	f.sent = nil
	f.inFront = false
	f.Tap(1, 0)
	f.MoveTo(image.Pt(150, 150))
	f.ClickRight()
	if len(f.sent) != 0 {
		t.Errorf("sent = %v, want nothing", f.sent)
	}
}

func TestGuardKeepsMovesInsideTheArea(t *testing.T) {
	f := newGuardFixture()
	f.MoveTo(image.Pt(50, 150))
	f.ClickRight()
	f.area = nil
	f.MoveTo(image.Pt(150, 150))
	if len(f.sent) != 0 {
		t.Errorf("sent = %v, want nothing", f.sent)
	}
}

func TestGuardDetectsTakeoverBeforeFirstWaypoint(t *testing.T) {
	f := newGuardFixture()
	f.cursor = image.Pt(150, 150)
	if _, _, ok := f.BeginCursor(); !ok {
		t.Fatal("expected a plan to start inside the window")
	}
	f.cursor.X += 20 // a hand on the mouse
	if f.MoveTo(image.Pt(160, 150)) == nil || f.takeovers != 1 || len(f.sent) != 0 {
		t.Fatalf("takeover did not stop the waypoint: sent=%v takeovers=%d", f.sent, f.takeovers)
	}
}

func TestGuardHandsOverWhenTheUserMovesTheCursor(t *testing.T) {
	tests := []struct {
		name string
		act  func(*guardFixture)
	}{
		{"before the click", func(f *guardFixture) { f.ClickRight() }},
		{"before the next move", func(f *guardFixture) { f.MoveTo(image.Pt(150, 150)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGuardFixture()
			f.MoveTo(image.Pt(150, 150))
			f.sent = nil
			f.cursor = image.Pt(170, 150) // a hand on the mouse
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

// A pointer nudged by a pixel or two, by a sensitive mouse or the game, is
// still the app's: the run goes on and the click goes out.
func TestGuardToleratesASmallDrift(t *testing.T) {
	f := newGuardFixture()
	f.MoveTo(image.Pt(150, 150))
	f.cursor = image.Pt(152, 149)
	if f.ClickRight() != nil || f.takeovers != 0 {
		t.Fatalf("a 2 px drift stopped the click: takeovers=%d", f.takeovers)
	}
	f.cursor = image.Pt(150+takeoverSlack+1, 150)
	if f.ClickRight() == nil || f.takeovers != 1 {
		t.Fatalf("a move past the slack did not hand over: takeovers=%d", f.takeovers)
	}
}

func TestCameraDragRequiresRoomAndFocus(t *testing.T) {
	f := newGuardFixture()
	f.cursor = image.Pt(90, 150)
	if attempted, moved := f.DragCamera(12); attempted || moved || len(f.sent) != 0 {
		t.Fatal("drag was attempted with the pointer outside the game")
	}
	f.cursor = image.Pt(150, 150)
	if attempted, moved := f.DragCamera(12); !attempted || !moved || len(f.sent) != 1 {
		t.Fatal("drag did not run inside the focused game")
	}
	f.inFront = false
	if attempted, moved := f.DragCamera(12); !attempted || moved || len(f.sent) != 1 {
		t.Fatal("lost focus should block the drag and its fallback")
	}
	f.inFront = true
	f.CanDrag = func() bool { return false }
	if attempted, _ := f.DragCamera(12); attempted {
		t.Fatal("a Board that cannot drag should leave the move to the keys")
	}
}

// A refused click says why, so the run can stop with that reason.
func TestGuardClickSaysWhy(t *testing.T) {
	tests := []struct {
		name string
		set  func(f *guardFixture)
		want string
	}{
		{"no move before", func(f *guardFixture) { f.ResetCursor() }, "the pointer was not on the bobber"},
		{"game behind", func(f *guardFixture) { f.inFront = false }, "the game was not in front"},
		{"window shrank", func(f *guardFixture) { r := image.Rect(100, 100, 120, 120); f.area = &r }, "the bobber was outside the game window"},
		{"mouse moved", func(f *guardFixture) { f.cursor = image.Pt(170, 150) }, "the mouse was moved"},
		{"board failed", func(f *guardFixture) { f.Click = func() error { return errors.New("board said no") } }, "board said no"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newGuardFixture()
			f.MoveTo(image.Pt(150, 150))
			tt.set(f)
			if err := f.ClickRight(); err == nil || err.Error() != tt.want {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
