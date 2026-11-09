package cursormotion

import (
	"image"
	"math"
	"math/rand/v2"
	"time"
)

// maxDefaultPath bounds a DefaultPath, overshoot and correction included.
const maxDefaultPath = 640 * time.Millisecond

// DefaultPath plans a bounded reach when no recording-derived profile is loaded.
// No two reaches look alike: the path bends to a random side, wobbles, trembles
// a little until it slows near the end, and now and then overshoots the target
// and comes back.
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
	if distance > 60 && rand.IntN(3) == 0 {
		// Overshoot along the reach by a few percent, a little to one side.
		past, side := distance*(0.03+0.05*rand.Float64()), distance*0.03*(2*rand.Float64()-1)
		end = clamp(image.Pt(target.X+int(math.Round((dx*past-dy*side)/distance)),
			target.Y+int(math.Round((dy*past+dx*side)/distance))))
	}
	duration := time.Duration(min(480, 70+0.45*distance+float64(rand.IntN(70)))) * time.Millisecond
	steps := reach(start, end, duration, 0, distance, clamp)
	if end == target {
		return append(steps, Step{After: duration, Point: target})
	}
	steps = append(steps, Step{After: duration, Point: end})
	back := time.Duration(60+rand.IntN(80)) * time.Millisecond
	steps = append(steps, reach(end, target, back, duration, 0, clamp)...)
	return append(steps, Step{After: duration + back, Point: target})
}

// reach returns the waypoints from start toward end over duration, offset in
// time by after, without the end point. sway, the reach's length, sets how far
// it may bend and tremble; 0 draws a plain eased line.
func reach(start, end image.Point, duration, after time.Duration, sway float64, clamp func(image.Point) image.Point) []Step {
	dx, dy := float64(end.X-start.X), float64(end.Y-start.Y)
	length := max(1, math.Hypot(dx, dy))
	bend := min(48, sway*0.22) * (2*rand.Float64() - 1)
	wave := min(14, sway*0.07) * (2*rand.Float64() - 1)
	tremor := min(2.5, sway*0.01)
	var steps []Step
	last := start
	for at := time.Duration(0); ; {
		at += time.Duration(10+rand.IntN(12)) * time.Millisecond
		if at >= duration-8*time.Millisecond {
			break
		}
		t := float64(at) / float64(duration)
		progress := t * t * t * (10 + t*(-15+6*t))
		calm := 1 - progress // the tremor fades as the hand slows
		lateral := bend*4*t*(1-t) + wave*math.Sin(2*math.Pi*t) +
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
