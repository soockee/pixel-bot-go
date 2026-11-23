package action

import (
	"errors"
	"image"
	"log/slog"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// Guard is the live fishing.Input: it sends input to the Board and drops
// anything that could land in the wrong place. Its methods run on the FSM
// goroutine, so its fields need no locking.
type Guard struct {
	Logger    *slog.Logger
	InFront   func() bool                // the selected window has focus
	Area      func() *image.Rectangle    // capture area in screen coordinates
	CursorPos func() (x, y int, ok bool) // where the pointer is now
	// The Board's input, sent when allowed.
	Press   func(vk byte, hold time.Duration) error
	Move    func(x, y int) error
	Click   func() error
	CanDrag func() bool // the Board can drag and the left button is free
	Drag    func(dx int) error

	target    image.Point // where the last Move left the pointer
	hasTarget bool
}

// Tap sends the key if it can reach the game.
func (g *Guard) Tap(vk byte, hold time.Duration) error {
	if !g.InFront() {
		g.Logger.Warn("input dropped: selected window is not in front", "action", "press_key")
		return fishing.ErrNotInFront
	}
	return g.Press(vk, hold)
}

// MoveTo moves the pointer to a waypoint, or says why it did not.
func (g *Guard) MoveTo(p image.Point) error {
	if g.userMoved() {
		return fishing.ErrMouseMoved
	}
	g.hasTarget = false
	if !g.InFront() {
		g.Logger.Warn("input dropped: window not in front", "x", p.X, "y", p.Y)
		return fishing.ErrNotInFront
	}
	if area := g.Area(); area == nil || !p.In(*area) {
		g.Logger.Warn("input dropped: move outside the capture area", "x", p.X, "y", p.Y)
		return errors.New("the pointer path left the game window")
	}
	if err := g.Move(p.X, p.Y); err != nil {
		return err
	}
	g.target, g.hasTarget = p, true
	return nil
}

// ResetCursor gives up the previous run's pointer ownership.
func (g *Guard) ResetCursor() { g.hasTarget = false }

// BeginCursor claims the pointer at its current position before scheduling the
// first waypoint, so a move by the user during that wait also takes over.
func (g *Guard) BeginCursor() (image.Point, image.Rectangle, error) {
	if !g.InFront() {
		return image.Point{}, image.Rectangle{}, fishing.ErrNotInFront
	}
	area := g.Area()
	if area == nil {
		return image.Point{}, image.Rectangle{}, errors.New("the game window is not known")
	}
	x, y, ok := g.CursorPos()
	p := image.Pt(x, y)
	if g.cursorTaken(p, ok) {
		return image.Point{}, image.Rectangle{}, fishing.ErrMouseMoved
	}
	if !ok || !p.In(*area) {
		return image.Point{}, image.Rectangle{}, errors.New("the pointer is outside the game window")
	}
	g.target, g.hasTarget = p, true
	return p, *area, nil
}

// DragCamera keeps the drag inside the game and checks for user takeover
// before moving. The game may relocate the pointer on release. attempted is
// false when no drag fits, so the caller can walk instead; otherwise err says
// why the drag did not happen.
func (g *Guard) DragCamera(dx int) (attempted bool, err error) {
	if !g.CanDrag() {
		return false, nil
	}
	if !g.InFront() {
		return true, fishing.ErrNotInFront
	}
	area := g.Area()
	x, y, ok := g.CursorPos()
	start := image.Pt(x, y)
	if g.cursorTaken(start, ok) {
		return true, fishing.ErrMouseMoved
	}
	if area == nil || !ok {
		return false, nil
	}
	inner := area.Inset(8)
	if !start.Add(image.Pt(3*dx, 0)).In(inner) || !start.In(inner) {
		return false, nil
	}
	g.target, g.hasTarget = start, true
	defer g.ResetCursor()
	switch {
	case !g.InFront():
		return true, fishing.ErrNotInFront
	case g.userMoved() || !g.CanDrag(): // a held left button is the person using the mouse
		return true, fishing.ErrMouseMoved
	}
	if err := g.Drag(dx); err != nil {
		g.Logger.Error("ESP32 camera drag failed", "error", err)
		return true, err
	}
	return true, nil
}

// ClickRight clicks if the pointer is still where MoveTo left it, or
// says why it did not.
func (g *Guard) ClickRight() error {
	if err := g.canClick(); err != nil {
		return err
	}
	return g.Click()
}

func (g *Guard) canClick() error {
	if !g.hasTarget {
		g.Logger.Warn("input dropped: no target", "action", "click_right")
		return errors.New("the pointer was not on the bobber")
	}
	if !g.InFront() {
		g.Logger.Warn("input dropped: window not in front", "action", "click_right")
		return fishing.ErrNotInFront
	}
	area := g.Area()
	if area == nil || !g.target.In(*area) {
		g.Logger.Warn("input dropped: target is outside the window", "action", "click_right")
		g.hasTarget = false
		return errors.New("the bobber was outside the game window")
	}
	if g.userMoved() {
		return fishing.ErrMouseMoved
	}
	return nil
}

// userMoved reports whether the pointer left the last target, which means the
// user has taken over.
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
	return true
}
