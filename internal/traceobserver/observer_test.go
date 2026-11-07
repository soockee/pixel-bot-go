package traceobserver

import (
	"math"
	"testing"
)

func TestTimingHazardAccountsForCensoredWait(t *testing.T) {
	var model timingModel
	model.Learn(timingSample{context: 0, delay: 25, action: true})
	model.Learn(timingSample{context: 0, delay: 50, action: false})
	row := model[0]
	if row[0] != (hazardBin{Exposures: 2}) || row[1] != (hazardBin{Exposures: 2, Actions: 1}) || row[2] != (hazardBin{}) {
		t.Fatalf("wrong risk sets: %+v", row[:3])
	}
	if got, want := model.surprise(timingSample{0, 25, true}), -math.Log(.75*.5); math.Abs(got-want) > 1e-12 {
		t.Fatalf("action surprise %.6f, want %.6f", got, want)
	}
	if got, want := model.surprise(timingSample{0, 50, false}), -math.Log(.75*.5); math.Abs(got-want) > 1e-12 {
		t.Fatalf("censored surprise %.6f, want %.6f", got, want)
	}
}

func TestSequenceRecurrenceAtTwoScales(t *testing.T) {
	var tokens []sequenceToken
	for i := range 16 {
		tokens = append(tokens, sequenceToken{i%4 == 3, OutcomeOK, i % 4})
	}
	if got := recurrence(tokens, 4); got != 1 {
		t.Fatalf("short recurrence = %v, want 1", got)
	}
	if got := recurrence(tokens, 8); got != 1 {
		t.Fatalf("long recurrence = %v, want 1", got)
	}
	for i := 8; i < 16; i++ {
		tokens[i].outcome = OutcomeFail
	}
	if got := recurrence(tokens, 8); got != 0 {
		t.Fatalf("long recurrence = %v, want 0 after outcomes change", got)
	}
}

func TestShadowClockExpiresAndUsesFixedDenominator(t *testing.T) {
	var clock detectorClock
	clock.tick(0, 100, .5)
	clock.set(25, .75)
	for _, now := range []int64{50, 100, 150, 200} {
		clock.tick(now, 100, .5)
	}
	clock.set(225, .75)
	clock.tick(250, 100, .5)
	got := clock.result
	if got.Ticks != 6 || got.ScoredTicks != 3 || got.FlaggedTicks != 3 || got.Episodes != 2 ||
		got.FirstAlertMs == nil || *got.FirstAlertMs != 50 {
		t.Fatalf("unexpected fixed-clock result: %+v", got)
	}
}
