package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const (
	featureTiming   = "reel_interval_regularity"
	featureSequence = "repeated_attempt_sequence"
	featureRetries  = "retry_burst"
)

type traceFeedbackConfig struct {
	WindowActions       int     `json:"window_actions"`
	IntervalToleranceMs int64   `json:"interval_tolerance_ms"`
	RetryWindowMs       int64   `json:"retry_window_ms"`
	RetryBurstCount     int     `json:"retry_burst_count"`
	Threshold           float64 `json:"threshold"`
	PacingHoldMs        int64   `json:"pacing_hold_ms"`
	RetryHoldMs         int64   `json:"retry_hold_ms"`
}

func (c traceFeedbackConfig) validate() error {
	if c.WindowActions < 5 || c.IntervalToleranceMs < 0 || c.RetryWindowMs <= 0 ||
		c.RetryBurstCount < 2 || c.RetryBurstCount > c.WindowActions || c.Threshold <= 0 || c.Threshold > 1 ||
		c.PacingHoldMs <= 0 || c.RetryHoldMs <= 0 {
		return fmt.Errorf("config: invalid trace_feedback settings")
	}
	return nil
}

type traceFeatures struct {
	Timing   float64 `json:"reel_interval_regularity"`
	Sequence float64 `json:"repeated_attempt_sequence"`
	Retries  float64 `json:"retry_burst"`
}

func (f traceFeatures) strongest() (string, float64) {
	switch {
	case f.Retries >= f.Timing && f.Retries >= f.Sequence:
		return featureRetries, f.Retries
	case f.Timing >= f.Sequence:
		return featureTiming, f.Timing
	default:
		return featureSequence, f.Sequence
	}
}

type traceAdjustment struct {
	Scope           string `json:"scope"`
	PreviousUntilMs int64  `json:"previous_until_ms"`
	UntilMs         int64  `json:"until_ms"`
}

type traceSignal struct {
	TMs        int64            `json:"t_ms"`
	Feature    string           `json:"feature"`
	Score      float64          `json:"score"`
	Threshold  float64          `json:"threshold"`
	Features   traceFeatures    `json:"features"`
	EventIDs   []int            `json:"event_ids"`
	Adjustment *traceAdjustment `json:"adjustment,omitempty"`
}

type traceScore struct {
	ScoredWindows  int           `json:"scored_windows"`
	FlaggedWindows int           `json:"flagged_windows"`
	Scorable       bool          `json:"scorable"`
	PeakScore      float64       `json:"peak_score"`
	PeakFeatures   traceFeatures `json:"peak_features"`
	FirstSignalMs  *int64        `json:"first_signal_ms"`
	Signals        []traceSignal `json:"signals"`
}

type traceScorer struct {
	cfg    traceFeedbackConfig
	window []Record
	above  bool
	result traceScore
}

// observe consumes only attempts that have already been emitted. Feedback on a
// crossing is delivered synchronously so it can affect only future attempts.
func (s *traceScorer) observe(r Record, respond func(traceSignal) *traceAdjustment) {
	if r.Action != actionAttempt {
		return
	}
	if len(s.window) == s.cfg.WindowActions {
		s.window = slices.Delete(s.window, 0, 1)
	}
	s.window = append(s.window, r)
	features := s.features(r.TMs)
	feature, score := features.strongest()
	if len(s.window) >= 5 || score >= s.cfg.Threshold {
		s.result.Scorable = true
		s.result.ScoredWindows++
		if score >= s.cfg.Threshold {
			s.result.FlaggedWindows++
		}
	}
	s.result.PeakScore = max(s.result.PeakScore, score)
	peak := &s.result.PeakFeatures
	peak.Timing = max(peak.Timing, features.Timing)
	peak.Sequence = max(peak.Sequence, features.Sequence)
	peak.Retries = max(peak.Retries, features.Retries)

	crossed := !s.above && score >= s.cfg.Threshold
	s.above = score >= s.cfg.Threshold
	if !crossed {
		return
	}

	signal := traceSignal{TMs: r.TMs, Feature: feature, Score: score, Threshold: s.cfg.Threshold, Features: features}
	for _, action := range s.window {
		if feature == featureRetries && (action.Attempt <= 1 || r.TMs-action.TMs > s.cfg.RetryWindowMs) {
			continue
		}
		if !slices.Contains(signal.EventIDs, action.EventID) {
			signal.EventIDs = append(signal.EventIDs, action.EventID)
		}
	}
	if respond != nil {
		signal.Adjustment = respond(signal)
	}
	if s.result.FirstSignalMs == nil {
		s.result.FirstSignalMs = &signal.TMs
	}
	s.result.Signals = append(s.result.Signals, signal)
}

func (s *traceScorer) features(now int64) traceFeatures {
	var f traceFeatures
	var similar, repeated, retries int
	window := s.window
	for i, action := range window {
		if action.Attempt > 1 && now-action.TMs <= s.cfg.RetryWindowMs {
			retries++
		}
		if i >= 2 {
			gap := action.TMs - window[i-1].TMs
			previousGap := window[i-1].TMs - window[i-2].TMs
			if gap >= previousGap-s.cfg.IntervalToleranceMs && previousGap >= gap-s.cfg.IntervalToleranceMs {
				similar++
			}
		}
		// Compare adjacent pairs of first-attempt/retry symbols. A pair of
		// ordinary first attempts alone does not count as a repeating motif.
		if i >= 3 {
			a, b := action.Attempt > 1, window[i-1].Attempt > 1
			if (a || b) && a == (window[i-2].Attempt > 1) && b == (window[i-3].Attempt > 1) {
				repeated++
			}
		}
	}
	if len(window) >= 5 {
		f.Timing = float64(similar) / float64(len(window)-2)
	}
	if len(window) >= 4 {
		f.Sequence = float64(repeated) / float64(len(window)-3)
	}
	f.Retries = math.Min(1, float64(retries)/float64(s.cfg.RetryBurstCount))
	return f
}

func (c *boundedBackoff) applySignal(cfg traceFeedbackConfig, signal traceSignal) *traceAdjustment {
	scope, delay, until := "all_attempts", cfg.PacingHoldMs, &c.feedbackUntil
	if signal.Feature == featureRetries {
		scope, delay, until = "retries", cfg.RetryHoldMs, &c.retryUntil
	}
	next := signal.TMs + min(delay, c.cfg.MaxBackoffMs)
	if next <= *until {
		return nil
	}
	adjustment := &traceAdjustment{Scope: scope, PreviousUntilMs: *until, UntilMs: next}
	*until = next
	return adjustment
}

const feedbackSuffix = "+feedback"

func simulateWithFeedback(sc Scenario, cc ControllerConfig, cfg traceFeedbackConfig, observers ...func(Record)) ([]Record, *traceScore, error) {
	name, enabled := cc.Name, false
	if base, found := strings.CutSuffix(name, feedbackSuffix); found {
		cc.Name, enabled = base, true
	}
	controller := newController(cc, sc.Seed)
	scorer := traceScorer{cfg: cfg}
	var respond func(traceSignal) *traceAdjustment
	if enabled {
		backoff, ok := controller.(*boundedBackoff)
		if !ok {
			return nil, nil, fmt.Errorf("feedback requires a backoff controller")
		}
		respond = func(signal traceSignal) *traceAdjustment { return backoff.applySignal(cfg, signal) }
	}
	trace, err := simulateObserved(sc, controller, name, func(r Record) {
		for _, observe := range observers {
			observe(r)
		}
		scorer.observe(r, respond)
	})
	return trace, &scorer.result, err
}

var traceMetricDefs = []struct {
	name string
	get  func(Metrics) (float64, bool)
}{
	{"flagged_run", func(m Metrics) (float64, bool) {
		if m.TraceScore.FirstSignalMs != nil {
			return 1, true
		}
		return 0, m.TraceScore.Scorable
	}},
	{"unscorable_run", func(m Metrics) (float64, bool) {
		if !m.TraceScore.Scorable {
			return 1, true
		}
		return 0, true
	}},
	{"signal_count", func(m Metrics) (float64, bool) {
		return float64(len(m.TraceScore.Signals)), m.TraceScore.Scorable
	}},
	{"flagged_window_rate", func(m Metrics) (float64, bool) {
		if !m.TraceScore.Scorable {
			return 0, false
		}
		return float64(m.TraceScore.FlaggedWindows) / float64(m.TraceScore.ScoredWindows), true
	}},
	{"first_signal_ms", func(m Metrics) (float64, bool) { return optional(m.TraceScore.FirstSignalMs) }},
	{"peak_trace_score", func(m Metrics) (float64, bool) { return m.TraceScore.PeakScore, m.TraceScore.Scorable }},
}

type traceComparison struct {
	Baseline  string       `json:"baseline"`
	Treatment string       `json:"treatment"`
	Rows      []Comparison `json:"rows"`
}

func traceReport(metrics []Metrics) string {
	type totals struct {
		runs, scorable, flagged, signals, adjusted, events, completed, missed, timedOut int
	}
	byController := map[string]*totals{}
	var names []string
	for _, m := range metrics {
		t := byController[m.Controller]
		if t == nil {
			t = &totals{}
			byController[m.Controller] = t
			names = append(names, m.Controller)
		}
		t.runs++
		t.events += m.Events
		t.completed += m.Completed
		t.missed += m.Missed
		t.timedOut += m.TimedOut
		if m.TraceScore.Scorable {
			t.scorable++
		}
		if m.TraceScore.FirstSignalMs != nil {
			t.flagged++
		}
		t.signals += len(m.TraceScore.Signals)
		for _, signal := range m.TraceScore.Signals {
			if signal.Adjustment != nil {
				t.adjusted++
			}
		}
	}
	var b strings.Builder
	b.WriteString("# Streaming trace evaluation\n\n")
	b.WriteString("Signals mark threshold crossings on simulated reel attempts. Flag rates use scorable runs. Time to first signal uses flagged runs. Completion uses every bite. Feedback starts after the first signal. Compare signal counts and flagged-window rates to evaluate what happens afterward.\n\n")
	b.WriteString("| Profile | Runs | Scorable | Flagged | Signals | Adjustments | Completion | Missed | Timed out |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, name := range names {
		t := byController[name]
		flagged := fmt.Sprintf("%d", t.flagged)
		if t.scorable > 0 {
			flagged = fmt.Sprintf("%d (%.2f%%)", t.flagged, 100*float64(t.flagged)/float64(t.scorable))
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %s | %d | %d | %.2f%% | %d | %d |\n",
			name, t.runs, t.scorable, flagged, t.signals, t.adjusted,
			100*float64(t.completed)/float64(t.events), t.missed, t.timedOut)
	}
	return b.String()
}
