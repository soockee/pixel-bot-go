package fishing

import (
	"image"
	"math"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// startIdle begins the between-catch activity once a cooldown is over. One
// time in four it is a small camera nudge, which the next nudge turns back, so
// the angle the spot needs drifts by one nudge at most. Otherwise it is a
// pointer path inside the game window: a short wander, or one time in
// swirlOdds a swirl round the whole window. The path runs as timed steps on
// the cooldown's deadline, so any transition ends it and no step of it can
// reach a cast, a watch or a reel. It moves the pointer only, never a button.
func (f *FishingFSM) startIdle() {
	roll := f.deps.Roll
	if roll(4) == 0 {
		dx := -f.idleNudge
		if dx == 0 {
			dx = 4 + roll(7)
			if roll(2) == 0 {
				dx = -dx
			}
		}
		if attempted, err := f.deps.Input.DragCamera(dx); attempted {
			if err == nil {
				f.idleNudge += dx
				f.logger.Info("between-catch camera nudge", "dx", dx)
			}
			if !f.pauseFor(err) { // a failed nudge costs nothing, unless the person took over
				f.arm(idleRest(roll))
			}
			return
		}
	}
	start, area, ok := f.deps.Pointer.Pos()
	if !ok || !start.In(area) {
		f.recast()
		return
	}
	kind, path := "wander", wanderPath(start, area, roll)
	if roll(swirlOdds) == 0 {
		kind, path = "swirl", swirlPath(start, area, roll)
	}
	f.logger.Info("between-catch cursor "+kind, "from", start, "steps", len(path))
	f.idlePath = path
	f.idleStep()
}

// idleStep moves the pointer to the next point of the idle path, and after
// the last one rests before the cast.
func (f *FishingFSM) idleStep() {
	p := f.idlePath[0]
	f.idlePath = f.idlePath[1:]
	// A step that does not land is skipped, unless the person took over or the game lost focus.
	if f.pauseFor(f.deps.Input.MoveTo(p)) {
		return
	}
	if len(f.idlePath) > 0 {
		f.arm(idleStepEvery)
		return
	}
	f.arm(idleRest(f.deps.Roll))
}

// idleRest is the pause between an idle move and the cast, 0.3 to 0.9 s.
func idleRest(roll func(int) int) time.Duration {
	return time.Duration(300+roll(601)) * time.Millisecond
}

const (
	idleStepEvery = 25 * time.Millisecond
	swirlOdds     = 4  // one idle pointer path in swirlOdds is a swirl
	swirlSpacing  = 40 // pixels between the points of a swirl, on average
)

// wanderPath is a pointer drift of 40 to 160 px from start along a bent
// curve, inside area. roll draws in [0, n).
func wanderPath(start image.Point, area image.Rectangle, roll func(int) int) []image.Point {
	distance := float64(40 + roll(121))
	angle := float64(roll(360)) * math.Pi / 180
	end := image.Pt(start.X+int(distance*math.Cos(angle)), start.Y+int(distance*math.Sin(angle)))
	return curve(start, clampTo(end, idleBounds(area)), area, roll)
}

// swirlPath leads from start onto an ellipse round the middle of area and
// follows it for three quarters of a turn to a turn and a half, either way
// round, its radius breathing a little: a hand circling the mouse while it waits.
func swirlPath(start image.Point, area image.Rectangle, roll func(int) int) []image.Point {
	in := idleBounds(area)
	c := in.Min.Add(in.Size().Div(2)).Add(image.Pt((roll(21)-10)*in.Dx()/100, (roll(21)-10)*in.Dy()/100))
	rx, ry := float64(in.Dx()*(25+roll(16))/100), float64(in.Dy()*(25+roll(16))/100)
	turn := 2 * math.Pi * float64(75+roll(76)) / 100
	if roll(2) == 0 {
		turn = -turn
	}
	from := float64(roll(360)) * math.Pi / 180
	on := func(a, r float64) image.Point {
		return image.Pt(c.X+int(rx*r*math.Cos(a)), c.Y+int(ry*r*math.Sin(a)))
	}
	path := curve(start, clampTo(on(from, 1), in), area, roll)
	// The arc length of an ellipse is about the angle times its mean radius.
	n := max(8, int(math.Abs(turn)*math.Hypot(rx, ry)/math.Sqrt2/swirlSpacing))
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		p := on(from+turn*t, 1+0.12*math.Sin(3*math.Pi*t)).Add(image.Pt(roll(3)-1, roll(3)-1))
		path = append(path, clampTo(p, in))
	}
	return path
}

// curve runs from start to end inside area, along a bend of up to a quarter
// of its length, slow at both ends, with a pixel of tremble on the way.
func curve(start, end image.Point, area image.Rectangle, roll func(int) int) []image.Point {
	d := end.Sub(start)
	bend := float64(roll(51)-25) / 100
	mid := image.Pt((start.X+end.X)/2-int(float64(d.Y)*bend), (start.Y+end.Y)/2+int(float64(d.X)*bend))
	n := 10 + roll(7) + int(math.Hypot(float64(d.X), float64(d.Y)))/30
	path := make([]image.Point, n)
	for i := range path {
		t := (1 - math.Cos(math.Pi*float64(i+1)/float64(n))) / 2
		x := (1-t)*(1-t)*float64(start.X) + 2*(1-t)*t*float64(mid.X) + t*t*float64(end.X)
		y := (1-t)*(1-t)*float64(start.Y) + 2*(1-t)*t*float64(mid.Y) + t*t*float64(end.Y)
		p := image.Pt(int(math.Round(x)), int(math.Round(y)))
		if i < n-1 {
			p = p.Add(image.Pt(roll(3)-1, roll(3)-1))
		}
		path[i] = clampTo(p, area)
	}
	return path
}

// idleBounds keeps where idle paths go a tenth of the window away from its
// edges, so the pointer ends well inside the game for the next aim.
func idleBounds(area image.Rectangle) image.Rectangle {
	return area.Inset(min(area.Dx(), area.Dy()) / 10)
}

func clampTo(p image.Point, r image.Rectangle) image.Point {
	return image.Pt(min(max(p.X, r.Min.X), r.Max.X-1), min(max(p.Y, r.Min.Y), r.Max.Y-1))
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
