package cursormotion

import (
	"image"
	"math"
	"math/rand/v2"
	"time"
)

// DefaultPath plans a bounded reach when no recording-derived profile is loaded.
func DefaultPath(start, target image.Point, bounds image.Rectangle, _ bool) []Step {
	if !start.In(bounds) || !target.In(bounds) {
		return nil
	}
	dx, dy := float64(target.X-start.X), float64(target.Y-start.Y)
	distance := math.Hypot(dx, dy)
	if distance < 4 {
		return []Step{{Point: target}}
	}
	duration := time.Duration(min(440, 65+0.40*distance+float64(rand.IntN(40)))) * time.Millisecond
	bend := min(22, distance*0.12) * (2*rand.Float64() - 1)
	var steps []Step
	last := start
	for at := time.Duration(0); ; {
		at += time.Duration(12+rand.IntN(9)) * time.Millisecond
		if at >= duration-8*time.Millisecond {
			break
		}
		t := float64(at) / float64(duration)
		progress := t * t * t * (10 + t*(-15+6*t))
		lateral := bend * 4 * t * (1 - t)
		point := image.Pt(
			min(bounds.Max.X-1, max(bounds.Min.X, start.X+int(math.Round(dx*progress-dy/distance*lateral)))),
			min(bounds.Max.Y-1, max(bounds.Min.Y, start.Y+int(math.Round(dy*progress+dx/distance*lateral)))),
		)
		if point != last && point != target {
			steps = append(steps, Step{After: at, Point: point})
			last = point
		}
	}
	return append(steps, Step{After: duration, Point: target})
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
