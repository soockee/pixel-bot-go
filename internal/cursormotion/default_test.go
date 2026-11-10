package cursormotion

import (
	"image"
	"testing"
	"time"
)

func TestDefaultPathStaysInClientAndEndsOnTarget(t *testing.T) {
	bounds := image.Rect(-300, 0, 0, 200)
	start, target := image.Pt(-299, 199), image.Pt(-1, 1)
	for range 200 {
		path := DefaultPath(start, target, bounds, false)
		if len(path) < 2 || path[len(path)-1].Point != target {
			t.Fatalf("bad endpoint in %v", path)
		}
		previous := time.Duration(-1)
		for _, step := range path {
			if !step.Point.In(bounds) || step.After <= previous || step.After > maxDefaultPath {
				t.Fatalf("invalid waypoint %+v after %v", step, previous)
			}
			previous = step.After
		}
	}
}

// A reach never pauses long enough to read as two in a dry-run trace, even
// while its slow end stays on one pixel.
func TestDefaultPathHasNoLongPause(t *testing.T) {
	bounds := image.Rect(0, 0, 3440, 1440)
	for i := range 20000 {
		start, target := image.Pt(i%3440, i%1440), image.Pt((i*7919)%3440, (i*104729)%1440)
		previous := time.Duration(0)
		for _, step := range DefaultPath(start, target, bounds, false) {
			if gap := step.After - previous; gap >= 50*time.Millisecond {
				t.Fatalf("%v to %v pauses %v before %+v", start, target, gap, step)
			}
			previous = step.After
		}
	}
}

// A long reach sometimes passes the target before it settles, and two reaches
// rarely share a waypoint.
func TestDefaultPathVaries(t *testing.T) {
	bounds := image.Rect(0, 0, 1000, 800)
	start, target := image.Pt(100, 700), image.Pt(700, 200)
	overshoots := 0
	for range 200 {
		for _, step := range DefaultPath(start, target, bounds, false) {
			if step.Point.X > target.X+5 { // further along than the target
				overshoots++
				break
			}
		}
	}
	if overshoots == 0 || overshoots == 200 {
		t.Errorf("%d of 200 reaches overshot, want some", overshoots)
	}
	a, b := DefaultPath(start, target, bounds, false), DefaultPath(start, target, bounds, false)
	same := 0
	for i := range min(len(a), len(b)) - 1 {
		if a[i].Point == b[i].Point {
			same++
		}
	}
	if same > len(a)/2 {
		t.Errorf("two reaches share %d of %d waypoints", same, len(a))
	}
}
