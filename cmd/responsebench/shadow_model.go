package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

type shadowConfig traceobserver.Config
type shadowModel = traceobserver.Model
type shadowThreshold = traceobserver.Threshold

func (s shadowConfig) validate(c Config) error {
	if s.FalseAlarmRate <= 0 || s.FalseAlarmRate >= 1 || s.TickMs <= 0 || s.ScoreAgeMs < s.TickMs {
		return fmt.Errorf("config: invalid shadow detector settings")
	}
	ranges := []Split{c.Splits["dev"], c.Splits["eval"], s.Train, s.Calibration}
	for i, a := range ranges {
		if a.Runs <= 0 || uint64(a.Runs-1) > math.MaxUint64-a.BaseSeed {
			return fmt.Errorf("config: invalid shadow seed range")
		}
		for _, b := range ranges[:i] {
			if a.BaseSeed <= b.BaseSeed+uint64(b.Runs-1) && b.BaseSeed <= a.BaseSeed+uint64(a.Runs-1) {
				return fmt.Errorf("config: shadow training, calibration, dev and eval seeds must be disjoint")
			}
		}
	}
	for _, name := range s.HoldoutFamilies {
		if !slices.ContainsFunc(c.Families, func(f Family) bool { return f.Name == name }) {
			return fmt.Errorf("config: unknown shadow holdout family %q", name)
		}
	}
	referenceFamilies := 0
	for _, f := range c.Families {
		if !slices.Contains(s.HoldoutFamilies, f.Name) {
			referenceFamilies++
		}
	}
	if referenceFamilies == 0 || float64(s.Calibration.Runs)*float64(referenceFamilies)+1 < 1/s.FalseAlarmRate {
		return fmt.Errorf("config: too few shadow calibration sessions for the requested false-alarm rate")
	}
	return nil
}

func calibrate(scores []float64, rate float64) (shadowThreshold, error) {
	rank := int(math.Ceil(float64(len(scores)+1) * (1 - rate)))
	if rank < 1 || rank > len(scores) {
		return shadowThreshold{}, fmt.Errorf("too few scorable reference sessions for shadow calibration")
	}
	slices.Sort(scores)
	cutoff := scores[rank-1]
	firstAbove := slices.IndexFunc(scores, func(score float64) bool { return score > cutoff })
	alerts := 0
	if firstAbove >= 0 {
		alerts = len(scores) - firstAbove
	}
	return shadowThreshold{Cutoff: cutoff, ReferenceRuns: len(scores), ReferenceAlerts: alerts}, nil
}

// referenceTrace uses a separate synthetic controller policy. It samples a
// session pace and varies first attempts and retries without exponential growth.
func referenceTrace(sc Scenario) ([]Record, error) {
	paces := [][2]int64{{5, 55}, {25, 125}, {70, 225}}
	pace := paces[sc.Seed%uint64(len(paces))]
	c := newController(ControllerConfig{
		Name: "synthetic-reference", Kind: kindBackoff, MinDelayMs: pace[0],
		MaxDelayMs: pace[1], BaseBackoffMs: 45, MaxBackoffMs: 400, MaxAttempts: 5,
	}, sc.Seed).(*boundedBackoff)
	return simulate(sc, &referenceController{c}, c.cfg.Name)
}

type referenceController struct{ *boundedBackoff }

func (c *referenceController) OnResult(now int64, id int, ok bool) {
	c.boundedBackoff.OnResult(now, id, ok)
	if ok {
		return
	}
	i := slices.IndexFunc(c.pending, func(p backoffItem) bool { return p.id == id })
	if i >= 0 {
		c.pending[i].ready = now + 70 + c.delay()
	}
}

func fitShadow(c Config) (*shadowModel, error) {
	model := &shadowModel{Version: 1, Config: traceobserver.Config(*c.Shadow)}
	for _, family := range c.Families {
		if slices.Contains(model.Config.HoldoutFamilies, family.Name) {
			continue
		}
		for i := range model.Config.Train.Runs {
			sc := generate(c, family, "shadow-train", model.Config.Train.BaseSeed+uint64(i))
			trace, err := referenceTrace(sc)
			if err != nil {
				return nil, err
			}
			var stream traceobserver.TimingStream
			for _, r := range trace {
				if sample, ok := stream.Observe(r); ok {
					model.Timing.Learn(sample)
				}
			}
		}
	}
	var timingScores, sequenceScores []float64
	for _, family := range c.Families {
		if slices.Contains(model.Config.HoldoutFamilies, family.Name) {
			continue
		}
		for i := range model.Config.Calibration.Runs {
			sc := generate(c, family, "shadow-calibration", model.Config.Calibration.BaseSeed+uint64(i))
			trace, err := referenceTrace(sc)
			if err != nil {
				return nil, err
			}
			result := scoreShadowTrace(model, sc, trace)
			if result.Timing.ScoredTicks > 0 {
				timingScores = append(timingScores, result.Timing.Peak)
			}
			if result.Sequence.ScoredTicks > 0 {
				sequenceScores = append(sequenceScores, result.Sequence.Peak)
			}
		}
	}
	var err error
	model.TimingThreshold, err = calibrate(timingScores, model.Config.FalseAlarmRate)
	if err != nil {
		return nil, fmt.Errorf("timing detector: %w", err)
	}
	model.SequenceThreshold, err = calibrate(sequenceScores, model.Config.FalseAlarmRate)
	if err != nil {
		return nil, fmt.Errorf("sequence detector: %w", err)
	}
	return model, nil
}
