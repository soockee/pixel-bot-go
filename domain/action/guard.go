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
	Move       func(x, y int)             //
	Click      func()                     //
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
	if g.userMoved() {
		return
	}
	g.hasTarget = false
	area := g.Area()
	if !g.InFront() || area == nil || !image.Pt(x, y).In(*area) {
		g.Logger.Warn("input dropped: move outside the capture area or window not in front", "x", x, "y", y)
		return
	}
	g.Move(x, y)
	g.target, g.hasTarget = image.Pt(x, y), true
}

// ClickRight clicks if the pointer is still where MoveCursor left it.
func (g *Guard) ClickRight() {
	if !g.hasTarget || !g.InFront() {
		g.Logger.Warn("input dropped: no target or window not in front", "action", "click_right")
		return
	}
	if !g.userMoved() {
		g.Click()
	}
}

// userMoved reports whether the pointer left the last target, which means the
// user has taken over. It runs OnTakeover when that happens.
func (g *Guard) userMoved() bool {
	if !g.hasTarget {
		return false
	}
	x, y, ok := g.CursorPos()
	if ok && image.Pt(x, y) == g.target {
		return false
	}
	g.Logger.Warn("input dropped: the user moved the cursor")
	g.hasTarget = false
	g.OnTakeover()
	return true
}
