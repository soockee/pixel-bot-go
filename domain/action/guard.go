package action

import (
	"image"
	"log/slog"
)

// Guard wraps real input and drops anything that could land in the wrong place.
// Its methods run on the FSM goroutine, so its fields need no locking.
type Guard struct {
	Logger     *slog.Logger
	InFront    func() bool                // the selected window has focus
	Area       func() *image.Rectangle    // capture area in screen coordinates
	CursorPos  func() (x, y int, ok bool) // where the pointer is now
	OnTakeover func()                     // the user moved the pointer
	Press      func(vk byte)              // input that is sent when allowed
	Move       func(x, y int) bool        //
	Click      func()                     //
	BeginHold  func() func()              // starts a timed right-button hold
	target     image.Point                // where the last Move left the pointer
	hasTarget  bool
}

// PressKey sends the key if the selected window has focus.
func (g *Guard) PressKey(vk byte) {
	if !g.InFront() {
		g.Logger.Warn("input dropped: selected window is not in front", "action", "press_key")
		return
	}
	g.Press(vk)
}

// MoveCursor moves the pointer to a point inside the capture area.
func (g *Guard) MoveCursor(x, y int) {
	g.TryMoveCursor(image.Pt(x, y))
}

// TryMoveCursor reports whether the guard and Windows accepted a waypoint.
func (g *Guard) TryMoveCursor(p image.Point) bool {
	if g.userMoved() {
		return false
	}
	g.hasTarget = false
	area := g.Area()
	if !g.InFront() || area == nil || !p.In(*area) {
		g.Logger.Warn("input dropped: move outside the capture area or window not in front", "x", p.X, "y", p.Y)
		return false
	}
	if !g.Move(p.X, p.Y) {
		return false
	}
	g.target, g.hasTarget = p, true
	return true
}

// ResetCursor gives up the previous run's pointer ownership.
func (g *Guard) ResetCursor() { g.hasTarget = false }

// BeginCursor claims the pointer at its current position before scheduling the
// first waypoint, so a move by the user during that wait also takes over.
func (g *Guard) BeginCursor() (image.Point, image.Rectangle, bool) {
	area := g.Area()
	if area == nil || !g.InFront() {
		return image.Point{}, image.Rectangle{}, false
	}
	x, y, ok := g.CursorPos()
	p := image.Pt(x, y)
	if g.cursorTaken(p, ok) || !ok || !p.In(*area) {
		return image.Point{}, image.Rectangle{}, false
	}
	g.target, g.hasTarget = p, true
	return p, *area, true
}

// ClickRight clicks if the pointer is still where MoveCursor left it.
func (g *Guard) ClickRight() {
	if g.canClick() {
		g.Click()
	}
}

// HoldRight checks the press. The caller must release even after focus loss.
func (g *Guard) HoldRight() func() {
	if !g.canClick() {
		return nil
	}
	return g.BeginHold()
}

func (g *Guard) canClick() bool {
	if !g.hasTarget || !g.InFront() {
		g.Logger.Warn("input dropped: no target or window not in front", "action", "click_right")
		return false
	}
	area := g.Area()
	if area == nil || !g.target.In(*area) {
		g.Logger.Warn("input dropped: target is outside the window", "action", "click_right")
		g.hasTarget = false
		return false
	}
	return !g.userMoved()
}

// userMoved reports whether the pointer left the last target, which means the
// user has taken over. It runs OnTakeover when that happens.
func (g *Guard) userMoved() bool {
	if !g.hasTarget {
		return false
	}
	x, y, ok := g.CursorPos()
	return g.cursorTaken(image.Pt(x, y), ok)
}

func (g *Guard) cursorTaken(p image.Point, ok bool) bool {
	if !g.hasTarget || ok && p == g.target {
		return false
	}
	g.Logger.Warn("input dropped: the user moved the cursor")
	g.hasTarget = false
	g.OnTakeover()
	return true
}
