package cursormotion

import (
	"image"
	"math"
	"math/rand/v2"
	"time"
)

// The default reach follows what the reviewed recordings show
// (docs/dev/reference/input-models.md): a reach takes about 0.2 to 0.9 s,
// longer for a longer way (Fitts' law), most of the way is a fast first movement
// and the rest slow homing,
// the pointer reports about every 10 ms, a reach bends to one side by about a
// tenth of its length, and many end with a short correction.
const (
	reachBase, reachPerBit = 230 * time.Millisecond, 105 * time.Millisecond // Fitts' law: base + per bit of log2(d/w+1)
	reachWidth             = 40.0                                           // the target's size in pixels, about a bobber's box
	minReach, maxReach     = 150 * time.Millisecond, 900 * time.Millisecond
	maxCorrection          = 220 * time.Millisecond
	// maxDefaultPath bounds a DefaultPath, correction included.
	maxDefaultPath = maxReach + maxCorrection
)

// DefaultPath plans a bounded reach when no recording-derived profile is loaded.
// No two reaches look alike: the time, the bend, where the hand is fastest and
// how the reach ends are drawn anew each time. About a third overshoot the
// target and a quarter stop short of it, and both then correct onto it.
func DefaultPath(start, target image.Point, bounds image.Rectangle, _ bool) []Step {
	if !start.In(bounds) || !target.In(bounds) {
		return nil
	}
	dx, dy := float64(target.X-start.X), float64(target.Y-start.Y)
	distance := math.Hypot(dx, dy)
	if distance < 4 {
		return []Step{{Point: target}}
	}
	clamp := func(p image.Point) image.Point {
		return image.Pt(min(bounds.Max.X-1, max(bounds.Min.X, p.X)), min(bounds.Max.Y-1, max(bounds.Min.Y, p.Y)))
	}
	end := target
	if distance > 40 {
		// along is how far the main reach goes, as a share of the way.
		along := 1.0
		switch roll := rand.Float64(); {
		case roll < 1.0/3:
			along = 1.03 + 0.06*rand.Float64()
		case roll < 1.0/3+1.0/4:
			along = 0.90 + 0.06*rand.Float64()
		}
		side := (along - 1) * (2*rand.Float64() - 1) // a miss is a little off to one side too
		end = clamp(image.Pt(start.X+int(math.Round(dx*along-dy*side)), start.Y+int(math.Round(dy*along+dx*side))))
	}
	bits := math.Log2(distance/reachWidth + 1)
	duration := time.Duration(float64(reachBase+time.Duration(bits*float64(reachPerBit))) * math.Exp(0.25*rand.NormFloat64()))
	duration = min(maxReach, max(minReach, duration))
	steps := reach(start, end, duration, 0, true, clamp)
	if end == target {
		return append(steps, Step{After: duration, Point: target})
	}
	steps = append(steps, Step{After: duration, Point: end})
	back := time.Duration(80+rand.IntN(int((maxCorrection-80*time.Millisecond)/time.Millisecond))) * time.Millisecond
	steps = append(steps, reach(end, target, back, duration, false, clamp)...)
	return append(steps, Step{After: duration + back, Point: target})
}

// reach returns the waypoints from start toward end over duration, offset in
// time by after, without the end point. A free reach bends, wobbles and
// trembles; a correction is a plain eased line.
func reach(start, end image.Point, duration, after time.Duration, free bool, clamp func(image.Point) image.Point) []Step {
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	length := max(1, math.Hypot(dx, dy))
	// A free reach is a fast primary movement over share of the way in the
	// first fast of the time, then slow homing for the rest.
	share, fast, bend, bendAt, wave, tremor := 1.0, 1.0, 0.0, 1.0, 0.0, 0.0
	if free {
		share, fast = 0.85+0.1*rand.Float64(), 0.3+0.45*rand.Float64()
		bend = min(0.6, 0.08*math.Exp(0.8*rand.NormFloat64())) * length
		if rand.IntN(2) == 0 {
			bend = -bend
		}
		bendAt = 0.6 + 0.8*rand.Float64() // where along the reach the bend is widest
		wave = min(14, length*0.05) * (2*rand.Float64() - 1)
		tremor = min(2.5, length*0.01)
	}
	var steps []Step
	last := start
	for at := time.Duration(0); ; {
		at += time.Duration(8000+rand.IntN(4001)) * time.Microsecond // a pointer reports about every 10 ms
		if at >= duration-6*time.Millisecond {
			break
		}
		t := float64(at) / float64(duration)
		progress := share * minJerk(t/fast)
		if t > fast {
			progress = share + (1-share)*minJerk((t-fast)/(1-fast))
		}
		calm := 1 - progress // the tremor fades as the hand slows
		lateral := bend*math.Sin(math.Pi*math.Pow(t, bendAt)) + wave*math.Sin(2*math.Pi*t) +
			tremor*calm*(2*rand.Float64()-1)
		along := tremor * calm * (2*rand.Float64() - 1)
		point := clamp(image.Pt(
			start.X+int(math.Round(dx*progress+(dx*along-dy*lateral)/length)),
			start.Y+int(math.Round(dy*progress+(dy*along+dx*lateral)/length)),
		))
		if point != last && point != end {
			steps = append(steps, Step{After: after + at, Point: point})
			last = point
		}
	}
	return steps
}

// minJerk is the smooth 0 to 1 progress of a single movement at time t of 1.
func minJerk(t float64) float64 {
	t = min(1, max(0, t))
	return t * t * t * (10 + t*(-15+6*t))
}

// ApproachPoint chooses whether to wait, park within the box, or move directly.
func ApproachPoint(center image.Point, box image.Rectangle) (image.Point, bool) {
	switch rand.IntN(3) {
	case 0:
		return center, false
	case 1:
		inner := box.Inset(4)
		if inner.Empty() {
			return center, false
		}
		// Pick a side of the visible marker without guessing outside it.
		offsets := [...]image.Point{
			image.Pt(inner.Min.X, center.Y), image.Pt(inner.Max.X-1, center.Y),
			image.Pt(center.X, inner.Min.Y), image.Pt(center.X, inner.Max.Y-1),
		}
		aim := offsets[rand.IntN(len(offsets))]
		return aim, aim.In(inner)
	default:
		return center, true
	}
}
