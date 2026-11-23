package fishing

import (
	"image"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// startGlances makes the pointer glance round the window while the bobber is
// watched, first 1.5 to 4 s after it reached where it waits.
func (f *FishingFSM) startGlances() {
	if len(f.cursorSteps) == 0 {
		at, area, ok := f.deps.Pointer.Pos()
		if !ok || !at.In(area) {
			return
		}
		f.cursorStart, f.cursorSteps = time.Now(), []CursorStep{{Point: at}}
	}
	f.watchIdle, f.restAim, f.away = true, f.cursorSteps[len(f.cursorSteps)-1].Point, false
	f.holdCursor(time.Duration(1500+f.deps.Roll(2501)) * time.Millisecond)
}

// glance plans the pointer's next move while the bobber is watched, along the
// same planned paths as the aim: out to a point anywhere in the game window,
// on to one or two more now and then, and back to where it waits beside the
// bobber, with a rest after each. The moves are the monitoring state's cursor
// steps, so a bite, a lost bobber or any other transition replaces them, and
// the reel plans its own path to the bobber from wherever the pointer is.
// glance reports false when it paused the run.
func (f *FishingFSM) glance() bool {
	roll := f.deps.Roll
	target, settle := f.restAim, true
	rest := time.Duration(2000+roll(4001)) * time.Millisecond
	if f.glancesLeft > 0 || !f.away {
		at, area, ok := f.deps.Pointer.Pos()
		if !ok || !at.In(area) {
			f.watchIdle = false
			return true
		}
		in := idleBounds(area)
		target, settle = image.Pt(in.Min.X+roll(in.Dx()), in.Min.Y+roll(in.Dy())), false
		rest = time.Duration(300+roll(901)) * time.Millisecond
		if f.away {
			f.glancesLeft--
		} else {
			f.glancesLeft = roll(3)
		}
	}
	steps, err := f.deps.Pointer.Plan(target, settle)
	if err != nil || len(steps) == 0 {
		if f.pauseFor(err) {
			return false
		}
		f.logger.Info("watching pointer move skipped", "error", err)
		f.watchIdle = false
		return true
	}
	f.away = target != f.restAim
	f.logger.Info("watching pointer move", "x", target.X, "y", target.Y, "back", !f.away)
	f.cursorStart, f.cursorSteps = time.Now(), steps
	f.holdCursor(rest)
	return true
}

// holdCursor keeps the pointer where its plan ends for d more, so the loop
// wakes then to plan the next glance.
func (f *FishingFSM) holdCursor(d time.Duration) {
	last := f.cursorSteps[len(f.cursorSteps)-1]
	f.cursorSteps = append(f.cursorSteps, CursorStep{After: last.After + d, Point: last.Point})
}

// idleBounds keeps glances a tenth of the window away from its edges, so the
// pointer stays well inside the game.
func idleBounds(area image.Rectangle) image.Rectangle {
	return area.Inset(min(area.Dx(), area.Dy()) / 10)
}

func (f *FishingFSM) activity(key, name string) bool {
	if key == "" {
		return false
	}
	vk, ok := config.KeyCode(key)
	if !ok {
		return false
	}
	if !f.deps.InFront() {
		f.transition(StateWaitingFocus)
		return false
	}
	if err := f.deps.Input.Tap(vk, 0); err != nil {
		f.inputFailed("could not reposition: ", err)
		return false
	}
	if f.deps.SimulateHolds {
		time.Sleep(interactHold) // held on the FSM clock, as the Board would
	}
	f.logger.Info(name, "key", key)
	return true
}

// WorldSignalArea is where Bobber Marker shows its in-world blocks, in the
// game window's client area.
var WorldSignalArea = image.Rect(8, 16, 26, 22)

// InWorld reports whether img, a capture of WorldSignalArea, shows the addon's
// magenta, green and magenta blocks. Addons do not run on the character
// screen, so without them the player is logged out or loading.
func InWorld(img *image.RGBA) bool {
	if img == nil || img.Bounds().Dx() < 18 || img.Bounds().Dy() < 6 {
		return false
	}
	near := func(a, b uint8) bool { return max(a, b)-min(a, b) <= 24 }
	o := img.Bounds().Min
	for i, rgb := range [3][3]uint8{{255, 0, 255}, {0, 255, 0}, {255, 0, 255}} {
		c := img.RGBAAt(o.X+3+i*6, o.Y+3)
		if !near(c.R, rgb[0]) || !near(c.G, rgb[1]) || !near(c.B, rgb[2]) {
			return false
		}
	}
	return true
}
