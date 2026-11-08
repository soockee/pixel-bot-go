package cursormotion

import (
	"image"
	"slices"
	"testing"
	"time"
)

func TestPlanStaysInsideClientAndEndsExactlyOnTarget(t *testing.T) {
	model := Model{Samples: []Sample{{
		Distance: 90,
		Points:   []Point{{}, {US: 80_000, Along: .5, Across: .22}, {US: 240_000, Along: .95, Across: .03}},
		Settle:   &Settle{PauseUS: 180_000, DurationUS: 80_000},
	}}}
	bounds := image.Rect(-120, -130, -20, -20)
	start, target := image.Pt(-105, -128), image.Pt(-21, -128)
	var corrections int
	for seed := uint64(1); seed <= 36; seed++ {
		path := model.Planner(seed)(start, target, bounds, true)
		plain := model.Planner(seed)(start, target, bounds, false)
		if len(path) < 3 || path[len(path)-1].Point != target ||
			!slices.Equal(path, model.Planner(seed)(start, target, bounds, true)) {
			t.Fatalf("unexpected path for seed %d: %+v", seed, path)
		}
		var last time.Duration
		for _, step := range path {
			if !step.Point.In(bounds) || step.After <= last {
				t.Fatalf("invalid step after %v: %+v", last, step)
			}
			last = step.After
		}
		if last > plain[len(plain)-1].After {
			corrections++
		}
	}
	if corrections == 0 || corrections == 36 {
		t.Fatalf("expected occasional corrections, got %d/36", corrections)
	}
	if path := model.Planner(1)(image.Point{}, target, bounds, true); path != nil {
		t.Fatalf("unexpected path starting outside the client: %+v", path)
	}
}

func TestPlanDistributesEndpointCorrectionOverTheApproach(t *testing.T) {
	model := Model{Samples: []Sample{{
		Distance: 100,
		Points:   []Point{{}, {US: 160_000, Along: .4}, {US: 321_000, Along: .8}},
	}}}
	target := image.Pt(100, 0)
	steps := model.Planner(1)(image.Point{}, target, image.Rect(0, 0, 200, 100), false)
	last := steps[len(steps)-1]
	previous := steps[len(steps)-2]
	if last.Point != target || last.After != 321*time.Millisecond {
		t.Fatalf("incorrect endpoint: %+v", last)
	}
	if gap, delta := last.After-previous.After, last.Point.X-previous.Point.X; gap < 12*time.Millisecond || delta > 6 {
		t.Fatalf("abrupt terminal step: %d pixels in %s", delta, gap)
	}
}
