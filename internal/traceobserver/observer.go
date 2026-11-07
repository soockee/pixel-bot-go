// Package traceobserver evaluates timing and sequence patterns in event streams.
package traceobserver

import (
	"math/bits"
	"slices"
)

// Result contains the latest totals for both detectors.
type Result struct {
	Timing   DetectorResult `json:"timing"`
	Sequence DetectorResult `json:"sequence"`
}

// Alert records an alert episode and the trace evidence available at its start.
type Alert struct {
	Detector  string  `json:"detector"`
	TMs       int64   `json:"t_ms"`
	Score     float64 `json:"score"`
	Threshold float64 `json:"threshold"`
	Trigger   Record  `json:"trigger"`
	CastIDs   []int   `json:"cast_ids"`
}

// DetectorResult summarizes coverage and alerts on the fixed clock.
type DetectorResult struct {
	Ticks        int     `json:"ticks"`
	ScoredTicks  int     `json:"scored_ticks"`
	FlaggedTicks int     `json:"flagged_ticks"`
	Episodes     int     `json:"episodes"`
	FirstAlertMs *int64  `json:"first_alert_ms"`
	Peak         float64 `json:"peak"`
}

type detectorClock struct {
	score    float64
	scoreAt  int64
	hasScore bool
	above    bool
	result   DetectorResult
}

func (c *detectorClock) set(now int64, score float64) {
	c.scoreAt, c.score, c.hasScore = now, score, true
}

func (c *detectorClock) tick(now, maxAge int64, cutoff float64) {
	c.result.Ticks++
	active := c.hasScore && now-c.scoreAt <= maxAge
	flagged := active && c.score > cutoff
	if active {
		c.result.ScoredTicks++
		c.result.Peak = max(c.result.Peak, c.score)
	}
	if flagged {
		c.result.FlaggedTicks++
		if !c.above {
			c.result.Episodes++
			if c.result.FirstAlertMs == nil {
				c.result.FirstAlertMs = &now
			}
		}
	}
	c.above = flagged
}

type sequenceToken struct {
	retry   bool
	outcome string
	gapBin  int
}

const (
	sequenceHistory     = 64
	sequencePersistence = 12
)

type sequenceStream struct {
	window          []sequenceToken
	previousAttempt int64
	gapBin          int
	sawAttempt      bool
	recent          []float64
}

func (s *sequenceStream) observe(r Record) (float64, bool) {
	if r.Action == ActionAttempt {
		if s.sawAttempt {
			gap := r.TMs - s.previousAttempt
			s.gapBin = int(gap / 25)
			if gap >= 1000 {
				s.gapBin = 39 + bits.Len64(uint64(gap/1000))
			}
		}
		s.previousAttempt, s.sawAttempt = r.TMs, true
	}
	if r.Action != ActionReply {
		return 0, false
	}
	if len(s.window) == sequenceHistory {
		s.window = slices.Delete(s.window, 0, 1)
	}
	s.window = append(s.window, sequenceToken{r.Attempt > 1, r.Outcome, s.gapBin})
	if len(s.window) < 8 {
		return 0, false
	}
	if len(s.recent) == sequencePersistence {
		s.recent = slices.Delete(s.recent, 0, 1)
	}
	s.recent = append(s.recent, max(recurrence(s.window, 4), recurrence(s.window, 8)))
	if len(s.recent) < sequencePersistence {
		return 0, false
	}
	return mean(s.recent), true
}

// recurrence averages up to three closest earlier matches to the current
// suffix. Comparisons cannot overlap the suffix itself.
func recurrence(tokens []sequenceToken, scale int) float64 {
	start := len(tokens) - scale
	if start < scale {
		return 0
	}
	best := [3]int{}
	seen := 0
	for offset := 0; offset+scale <= start; offset++ {
		matches := 0
		for i := range scale {
			if tokens[offset+i] == tokens[start+i] {
				matches++
			}
		}
		seen++
		for i, n := range best {
			if matches > n {
				best[i], matches = matches, n
			}
		}
	}
	return float64(best[0]+best[1]+best[2]) / float64(min(seen, len(best))*scale)
}

type shadowEvidence struct {
	timingIDs, sequenceIDs         []int
	timingTrigger, sequenceTrigger Record
	alerts                         []Alert
	limit                          int
}

// Observer scores an ordered event stream. Callers serialize access to it.
type Observer struct {
	evidence                   *shadowEvidence
	model                      *Model
	end, nextTick              int64
	timing                     TimingStream
	sequence                   sequenceStream
	timingClock, sequenceClock detectorClock
	surprises                  []float64
}

// New creates an observer with a fixed end time in elapsed milliseconds.
func New(model *Model, end int64) *Observer {
	return &Observer{model: model, end: end, timing: TimingStream{waits: make(map[int]pendingWait)}}
}

func appendEvidence(ids []int, id, limit int) []int {
	if len(ids) == limit {
		ids = slices.Delete(ids, 0, 1)
	}
	return append(ids, id)
}

func (e *shadowEvidence) alert(detector string, at int64, clock detectorClock, cutoff float64, trigger Record, ids []int) {
	var distinct []int
	for _, id := range ids {
		if !slices.Contains(distinct, id) {
			distinct = append(distinct, id)
		}
	}
	if e.limit > 0 && len(e.alerts) == e.limit {
		e.alerts = slices.Delete(e.alerts, 0, 1)
	}
	e.alerts = append(e.alerts, Alert{detector, at, clock.score, cutoff, trigger, distinct})
}

// Observe consumes one record in timestamp order.
func (s *Observer) Observe(r Record) {
	// Sample a tick only after all trace records at that time have arrived.
	s.advance(min(r.TMs, s.end))
	if r.TMs >= s.end {
		return
	}
	if sample, ok := s.timing.Observe(r); ok {
		if e := s.evidence; e != nil {
			e.timingIDs = appendEvidence(e.timingIDs, r.EventID, timingWindow)
			e.timingTrigger = r
		}
		if len(s.surprises) == timingWindow {
			s.surprises = slices.Delete(s.surprises, 0, 1)
		}
		s.surprises = append(s.surprises, s.model.Timing.surprise(sample))
		if len(s.surprises) == timingWindow {
			s.timingClock.set(r.TMs, mean(s.surprises))
		}
	}
	if e := s.evidence; e != nil && r.Action == ActionReply {
		e.sequenceIDs = appendEvidence(e.sequenceIDs, r.EventID, sequenceHistory+sequencePersistence-1)
		e.sequenceTrigger = r
	}
	if score, ok := s.sequence.observe(r); ok {
		s.sequenceClock.set(r.TMs, score)
	}
}

func (s *Observer) advance(until int64) {
	for s.nextTick < until {
		cfg := s.model.Config
		timingEpisodes, sequenceEpisodes := s.timingClock.result.Episodes, s.sequenceClock.result.Episodes
		s.timingClock.tick(s.nextTick, cfg.ScoreAgeMs, s.model.TimingThreshold.Cutoff)
		s.sequenceClock.tick(s.nextTick, cfg.ScoreAgeMs, s.model.SequenceThreshold.Cutoff)
		if e := s.evidence; e != nil {
			if s.timingClock.result.Episodes > timingEpisodes {
				e.alert("timing", s.nextTick, s.timingClock, s.model.TimingThreshold.Cutoff, e.timingTrigger, e.timingIDs)
			}
			if s.sequenceClock.result.Episodes > sequenceEpisodes {
				e.alert("sequence", s.nextTick, s.sequenceClock, s.model.SequenceThreshold.Cutoff, e.sequenceTrigger, e.sequenceIDs)
			}
		}
		s.nextTick += cfg.TickMs
	}
}

// Finish advances through the configured replay horizon and returns the totals.
func (s *Observer) Finish() Result {
	s.advance(s.end)
	return s.Result()
}

// EnableEvidence records alert details. A positive limit retains only the latest alerts.
func (s *Observer) EnableEvidence(limit int) {
	s.evidence = &shadowEvidence{limit: limit}
}

// Alerts returns a snapshot of the retained alert evidence.
func (s *Observer) Alerts() []Alert {
	if s.evidence == nil {
		return nil
	}
	return slices.Clone(s.evidence.alerts)
}

// Advance samples the fixed clock up to the supplied elapsed time.
func (s *Observer) Advance(elapsed int64) {
	s.advance(min(elapsed, s.end))
}

// Result returns the latest detector totals without advancing the clock.
func (s *Observer) Result() Result {
	return Result{s.timingClock.result, s.sequenceClock.result}
}

func mean(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
