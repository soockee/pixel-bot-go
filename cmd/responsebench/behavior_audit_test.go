package main

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cursormotion"
)

func TestReachHistoryNormalizesRepeats(t *testing.T) {
	start, target := image.Pt(20, 20), image.Pt(120, 20)
	first := plannedReach(start, target, []cursormotion.Step{
		{After: 20 * time.Millisecond, Point: image.Pt(70, 40)},
		{After: 40 * time.Millisecond, Point: target},
	})
	rotatedTarget := image.Pt(100, 300)
	rotated := plannedReach(image.Pt(100, 100), rotatedTarget, []cursormotion.Step{
		{After: 40 * time.Millisecond, Point: image.Pt(60, 200)},
		{After: 80 * time.Millisecond, Point: rotatedTarget},
	})
	var history reachHistory
	history.observe(first)
	history.observe(rotated)
	history.observe([]cursormotion.Point{{}})
	got := history.result()
	if got.Paths != 3 || got.Comparisons != 1 || got.MeanNearest == nil || *got.MeanNearest > 1e-9 ||
		got.MeanStepMS == nil || *got.MeanStepMS != 30 {
		t.Fatalf("reach audit: %+v", got)
	}
	history.observe([]cursormotion.Point{{}, {US: 20_000, Along: .5, Across: -.2}, {US: 40_000, Along: 1}})
	if distance := history.result().MeanNearest; distance == nil || *distance < .05 || math.IsNaN(*distance) {
		t.Fatalf("opposite curvature should increase distance, got %v", distance)
	}
}

func TestGameplayMotifsRespectGapsAndReelCounts(t *testing.T) {
	var h gameplayHistory
	for i := int64(0); i < 5; i++ {
		h.cast(i * 3_000_000)
		h.reel(i*3_000_000 + 1_000_000)
	}
	h.reel(13_100_000)
	h.cast(15_000_000)
	h.gap()
	h.cast(20_000_000)
	h.reel(21_000_000)
	h.cast(23_000_000)
	got := h.total.result()
	if got == nil || got.Cycles != 6 || got.ReeledCycles != 6 || got.ScoredMotifs != 2 ||
		got.RepeatedMotifs != 1 || got.RepeatFraction == nil || *got.RepeatFraction != .5 {
		t.Fatalf("gameplay audit: %+v", got)
	}
}
