package main

import (
	"image"
	"testing"
)

func TestCursorFitUsesReviewedReachAndLocalCorrection(t *testing.T) {
	bounds := image.Rect(0, 0, 200, 100)
	target := image.Pt(145, 40)
	bursts := []cursorBurst{
		{epoch: 1, bounds: bounds, points: []cursorPoint{
			{1_200_000, image.Pt(40, 40)}, {1_280_000, image.Pt(90, 49)}, {1_390_000, image.Pt(140, 42)},
		}},
		{epoch: 1, bounds: bounds, points: []cursorPoint{
			{1_640_000, image.Pt(142, 41)}, {1_690_000, image.Pt(145, 40)},
		}},
	}
	example, ok := extractApproach(bursts, 1, bounds, target, 1_000_000, 4_000_000)
	if !ok || example.human.DurationMS != 190 || example.sample.Settle == nil ||
		example.sample.Settle.PauseUS != 250_000 || example.sample.Settle.DurationUS != 50_000 {
		t.Fatalf("unexpected fit: %+v, %v", example, ok)
	}
	if _, ok := extractApproach(bursts, 2, bounds, target, 1_000_000, 4_000_000); ok {
		t.Fatal("approach should not cross a capture gap")
	}
	bursts[1].points[0].p = image.Pt(20, 20)
	if _, ok := extractApproach(bursts, 1, bounds, target, 1_000_000, 4_000_000); ok {
		t.Fatal("an unrelated move after the reach should exclude the approach")
	}
}
