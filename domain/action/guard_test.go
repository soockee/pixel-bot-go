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
		Press:      func(byte) bool { f.sent = append(f.sent, "press"); return true },
		Move: func(x, y int) bool {
			f.cursor = image.Pt(x, y)
			f.sent = append(f.sent, "move")
			return true
		},
		Click: func() bool { f.sent = append(f.sent, "click"); return true },
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

func TestGuardChecksTimedPressAndAllowsReleaseAfterFocusLoss(t *testing.T) {
	f := newGuardFixture()
	f.BeginHold = func() func() {
		f.sent = append(f.sent, "down")
		return func() { f.sent = append(f.sent, "up") }
	}
	f.MoveCursor(150, 150)
	release := f.HoldRight()
	if release == nil {
		t.Fatal("valid target should allow a hold")
	}
	f.inFront = false
	release()
	if f.HoldRight() != nil || !slices.Equal(f.sent, []string{"move", "down", "up"}) {
		t.Fatalf("unexpected input after focus loss: %v", f.sent)
	}
}

func TestGuardDetectsTakeoverBeforeFirstWaypoint(t *testing.T) {
	f := newGuardFixture()
	f.cursor = image.Pt(150, 150)
	if _, _, ok := f.BeginCursor(); !ok {
		t.Fatal("expected a plan to start inside the window")
	}
	f.cursor.X += 20 // a hand on the mouse
	if f.TryMoveCursor(image.Pt(160, 150)) || f.takeovers != 1 || len(f.sent) != 0 {
		t.Fatalf("takeover did not stop the waypoint: sent=%v takeovers=%d", f.sent, f.takeovers)
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
	f.MoveCursor(150, 150)
	f.cursor = image.Pt(152, 149)
	if !f.ClickRight() || f.takeovers != 0 {
		t.Fatalf("a 2 px drift stopped the click: takeovers=%d", f.takeovers)
	}
	f.cursor = image.Pt(150+takeoverSlack+1, 150)
	if f.ClickRight() || f.takeovers != 1 {
		t.Fatalf("a move past the slack did not hand over: takeovers=%d", f.takeovers)
	}
}

func TestCameraDragRequiresRoomAndFocus(t *testing.T) {
	f := newGuardFixture()
	f.cursor = image.Pt(90, 150)
	called := false
	drag := func(_ int, allowed func() bool) bool {
		called = true
		return allowed()
	}
	if attempted, moved := f.DragCamera(12, drag); attempted || moved || called {
		t.Fatal("drag was attempted with the pointer outside the game")
	}
	f.cursor = image.Pt(150, 150)
	if attempted, moved := f.DragCamera(12, drag); !attempted || !moved || !called {
		t.Fatal("drag did not run inside the focused game")
	}
	f.inFront = false
	called = false
	if attempted, moved := f.DragCamera(12, drag); !attempted || moved || called {
		t.Fatal("lost focus should block the drag and its fallback")
	}
}

func TestGuardRequiresFocusForKeyHold(t *testing.T) {
	f := newGuardFixture()
	f.BeginKeyHold = func(byte) func() { f.sent = append(f.sent, "hold"); return func() {} }
	if release := f.HoldKey(1); release == nil {
		t.Fatal("HoldKey refused a focused window")
	} else {
		release()
	}
	f.inFront = false
	if f.HoldKey(1) != nil {
		t.Error("HoldKey accepted an unfocused window")
	}
	if want := []string{"hold"}; !slices.Equal(f.sent, want) {
		t.Errorf("sent = %v, want %v", f.sent, want)
	}
}
