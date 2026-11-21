package fishing

import (
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// CheckActivityKeys rejects the WASD keys a reposition presses when another
// Go Fish action already uses one.
func CheckActivityKeys(cfg config.Config, parse func(string) (byte, bool)) error {
	if !cfg.Reposition {
		return nil
	}
	if parse == nil {
		return fmt.Errorf("optional game keys cannot be resolved")
	}
	used := make(map[byte]string)
	for _, binding := range []struct{ key, label string }{
		{cfg.CastKey, "Cast"}, {cfg.InteractKey, "Interact"}, {cfg.LogoutKey, "Logout"},
	} {
		if vk, ok := parse(binding.key); binding.key != "" && ok {
			used[vk] = binding.label
		}
	}
	for _, key := range "WASD" {
		if vk, ok := parse(string(key)); ok {
			if other, exists := used[vk]; exists {
				return fmt.Errorf("repositioning walks with %s, which is assigned to %s", string(key), other)
			}
		}
	}
	return nil
}

// idleMove plays one between-catch movement: now and then a small camera
// nudge where the input mode can drag, otherwise a cursor wander.
func (f *FishingFSM) idleMove() {
	if drag := f.cameraDrag(); drag != nil && rand.IntN(3) == 0 {
		dx := 4 + rand.IntN(7)
		if rand.IntN(2) == 0 {
			dx = -dx
		}
		if _, moved := drag(dx); moved {
			f.logger.Info("between-catch camera nudge", "dx", dx)
		}
		return
	}
	start := image.Pt(f.coordX, f.coordY)
	if f.actions.CursorPos != nil {
		if x, y, ok := f.actions.CursorPos(); ok {
			start = image.Pt(x, y)
		}
	}
	// shortcut: the path runs on the FSM loop, so a Stop waits for it (about
	// half a second); schedule its points as timed steps if that is too slow.
	for _, p := range wanderPath(start, rand.IntN) {
		if f.actions.CanAct != nil && !f.actions.CanAct() {
			return
		}
		f.actions.MoveCursor(p.X, p.Y)
		time.Sleep(wanderStep)
	}
	f.logger.Info("between-catch cursor wander", "from", start)
}

// cameraDrag is the ESP32 camera drag, nil without the board.
func (f *FishingFSM) cameraDrag() func(int) (bool, bool) {
	if f.hardwareKeyboard() {
		return f.actions.DragHardwareCamera
	}
	return nil
}

const wanderStep = 30 * time.Millisecond

// wanderPath is a cursor drift of 40 to 160 px from start along a bent curve,
// slow at both ends, with a pixel of tremble on the way. roll draws in [0, n).
func wanderPath(start image.Point, roll func(int) int) []image.Point {
	distance := float64(40 + roll(121))
	angle := float64(roll(360)) * math.Pi / 180
	end := image.Pt(start.X+int(distance*math.Cos(angle)), start.Y+int(distance*math.Sin(angle)))
	// The control point sits off the straight line by up to a quarter of its length.
	bend := float64(roll(51)-25) / 100 * distance
	mid := image.Pt((start.X+end.X)/2+int(-math.Sin(angle)*bend), (start.Y+end.Y)/2+int(math.Cos(angle)*bend))
	n := 10 + roll(7)
	path := make([]image.Point, n)
	for i := range path {
		t := (1 - math.Cos(math.Pi*float64(i+1)/float64(n))) / 2
		x := (1-t)*(1-t)*float64(start.X) + 2*(1-t)*t*float64(mid.X) + t*t*float64(end.X)
		y := (1-t)*(1-t)*float64(start.Y) + 2*(1-t)*t*float64(mid.Y) + t*t*float64(end.Y)
		path[i] = image.Pt(int(math.Round(x)), int(math.Round(y)))
		if i < n-1 {
			path[i] = path[i].Add(image.Pt(roll(3)-1, roll(3)-1))
		}
	}
	return path
}

func (f *FishingFSM) activity(key, name string) bool {
	if key == "" || f.actions.ParseVK == nil {
		return false
	}
	vk, ok := f.actions.ParseVK(key)
	if !ok {
		return false
	}
	if f.actions.CanAct != nil && !f.actions.CanAct() {
		f.transition(StateWaitingFocus)
		return false
	}
	if f.hardwareKeyboard() {
		if !f.tapKeyboard(vk, 0) {
			f.halt()
			return false
		}
	} else if f.actions.HoldInteract != nil {
		release := f.actions.HoldInteract(vk)
		if release == nil {
			return false
		}
		time.Sleep(interactHold)
		release()
	} else if !f.actions.PressKey(vk) {
		return false
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
