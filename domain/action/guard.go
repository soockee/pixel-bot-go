package action

import (
	"image"
	"log/slog"
)

// Guard wraps real input and drops anything that could land in the wrong place.
// Its methods run on the FSM goroutine, so its fields need no locking.
type Guard struct {
	Logger       *slog.Logger
	InFront      func() bool                // the selected window has focus
	Area         func() *image.Rectangle    // capture area in screen coordinates
	CursorPos    func() (x, y int, ok bool) // where the pointer is now
	OnTakeover   func()                     // the user moved the pointer
	Press        func(vk byte) bool         // input that is sent when allowed
	Move         func(x, y int) bool        //
	Click        func() bool                //
	BeginHold    func() func()              // starts a timed right-button hold
	BeginKeyHold func(byte) func()          // starts a timed key or side-button hold
	target       image.Point                // where the last Move left the pointer
	hasTarget    bool
}

// PressKey sends the key if it can reach the game.
func (g *Guard) PressKey(vk byte) bool {
	if !g.InFront() {
		g.Logger.Warn("input dropped: selected window is not in front", "action", "press_key")
		return false
	}
	return g.Press(vk)
}

// HoldKey starts a timed press if it can reach the game. The caller must
// release it even after focus loss.
func (g *Guard) HoldKey(vk byte) func() {
	if !g.InFront() {
		g.Logger.Warn("input dropped: selected window is not in front", "action", "hold_key")
		return nil
	}
	return g.BeginKeyHold(vk)
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

// DragCamera keeps the drag inside the game and checks for user takeover
// before moving. The game may relocate the pointer on release.
func (g *Guard) DragCamera(dx int, drag func(int, func() bool) bool) (attempted, moved bool) {
	if !g.InFront() {
		return true, false
	}
	area := g.Area()
	x, y, ok := g.CursorPos()
	start := image.Pt(x, y)
	if g.cursorTaken(start, ok) {
		return true, false
	}
	if area == nil || !ok {
		return false, false
	}
	inner := area.Inset(8)
	if !start.Add(image.Pt(3*dx, 0)).In(inner) || !start.In(inner) {
		return false, false
	}
	g.target, g.hasTarget = start, true
	defer g.ResetCursor()
	return true, drag(dx, func() bool { return g.InFront() && !g.userMoved() })
}

// ClickRight clicks if the pointer is still where MoveCursor left it.
func (g *Guard) ClickRight() bool {
	return g.canClick() && g.Click()
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

// takeoverSlack is how far, in pixels along either axis, the pointer may drift
// from where the guard left it before that counts as the user taking over. A
// sensitive mouse or the game can nudge it by a pixel or two between moves; a
// hand on the mouse moves it much further. A bobber's box is several times
// wider, so a click after such a drift still lands on it.
const takeoverSlack = 6

func (g *Guard) cursorTaken(p image.Point, ok bool) bool {
	d := p.Sub(g.target)
	if !g.hasTarget || ok && max(d.X, -d.X) <= takeoverSlack && max(d.Y, -d.Y) <= takeoverSlack {
		return false
	}
	g.Logger.Warn("input dropped: the user moved the cursor", "want_x", g.target.X, "want_y", g.target.Y, "x", p.X, "y", p.Y, "known", ok)
	g.hasTarget = false
	g.OnTakeover()
	return true
}
