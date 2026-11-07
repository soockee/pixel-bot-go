package main

import (
	"reflect"
	"testing"

	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

func TestShadowFitExcludesHoldoutAndRejectsOverlappingSeeds(t *testing.T) {
	cfg := testConfig(t)
	cfg.Shadow.Train.Runs = 5
	cfg.Shadow.Calibration.Runs = 8
	model, err := fitShadow(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, family := range cfg.Families {
		if family.Name == "mixed" {
			cfg.Families[i].MeanGapMs *= 3
		}
	}
	again, err := fitShadow(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(model, again) || model.TimingThreshold.ReferenceRuns != 32 ||
		model.SequenceThreshold.ReferenceRuns != 32 {
		t.Fatal("held-out workload affected fitting or calibration")
	}
	cfg.Shadow.Calibration.BaseSeed = cfg.Splits["eval"].BaseSeed
	if err := cfg.validate(); err == nil {
		t.Fatal("accepted overlapping calibration and eval seeds")
	}
}

func TestShadowObserversLeaveFeedbackTraceUnchanged(t *testing.T) {
	cfg := testConfig(t)
	sc := generate(cfg, cfg.Families[len(cfg.Families)-1], "eval", cfg.Splits["eval"].BaseSeed)
	var cc ControllerConfig
	for _, candidate := range cfg.Candidates {
		if candidate.Name == cfg.Frozen[1] {
			cc = candidate
		}
	}
	cc.Name += feedbackSuffix
	model := shadowModel{Config: traceobserver.Config(*cfg.Shadow)}
	before, scoreBefore, err := simulateWithFeedback(sc, cc, *cfg.TraceFeedback)
	if err != nil {
		t.Fatal(err)
	}
	observer := newShadowObserver(&model, sc)
	after, scoreAfter, err := simulateWithFeedback(sc, cc, *cfg.TraceFeedback, observer.Observe)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(scoreBefore, scoreAfter) {
		t.Fatal("silent observers changed controller actions or feedback")
	}
	result := observer.Finish()
	if result.Timing.ScoredTicks == 0 || result.Sequence.ScoredTicks == 0 ||
		!reflect.DeepEqual(result, scoreShadowTrace(&model, sc, after)) {
		t.Fatal("streaming detector results differ from offline replay")
	}
}
