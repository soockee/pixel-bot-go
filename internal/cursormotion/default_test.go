package cursormotion

import (
	"image"
	"testing"
	"time"
)

func TestDefaultPathStaysInClientAndEndsOnTarget(t *testing.T) {
	bounds := image.Rect(-300, 0, 0, 200)
	start, target := image.Pt(-299, 199), image.Pt(-1, 1)
	for range 100 {
		path := DefaultPath(start, target, bounds, false)
		if len(path) < 2 || path[len(path)-1].Point != target {
			t.Fatalf("bad endpoint in %v", path)
		}
		previous := time.Duration(-1)
		for _, step := range path {
			if !step.Point.In(bounds) || step.After <= previous || step.After > 440*time.Millisecond {
				t.Fatalf("invalid waypoint %+v after %v", step, previous)
			}
			previous = step.After
		}
	}
}
