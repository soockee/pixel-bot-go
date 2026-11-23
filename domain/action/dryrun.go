package action

import (
	"errors"
	"image"
	"log/slog"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// DryRun is the fishing.Input of a dry run: it logs each input as a
// "dry-run action" instead of sending it, so a run needs no Board and sends
// nothing. Like the Guard, it drops input while the game is not in front.
type DryRun struct {
	Logger  *slog.Logger
	InFront func() bool
	Area    func() *image.Rectangle // capture area in screen coordinates
	Moved   func(p image.Point, area image.Rectangle)
}

// Tap logs the key.
func (d DryRun) Tap(vk byte, hold time.Duration) error {
	if !d.InFront() {
		return fishing.ErrNotInFront
	}
	d.Logger.Info("dry-run action", "action", "press_key", "vk", vk, "hold_ms", hold.Milliseconds())
	return nil
}

// MoveTo logs a move to a point inside the capture area and reports it to Moved.
func (d DryRun) MoveTo(p image.Point) error {
	if !d.InFront() {
		return fishing.ErrNotInFront
	}
	area := d.Area()
	if area == nil || !p.In(*area) {
		return errors.New("the pointer path left the game window")
	}
	d.Logger.Info("dry-run action", "action", "move_cursor", "x", p.X, "y", p.Y)
	d.Moved(p, *area)
	return nil
}

// ClickRight logs the click.
func (d DryRun) ClickRight() error {
	if !d.InFront() {
		return fishing.ErrNotInFront
	}
	d.Logger.Info("dry-run action", "action", "click_right")
	return nil
}

// DragCamera never drags: a dry run has no Board, so a reposition walks.
func (d DryRun) DragCamera(int) (attempted, moved bool) { return false, false }
