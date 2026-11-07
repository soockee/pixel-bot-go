package app

import (
	"math"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/internal/playtrace"
	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

type liveTrace struct {
	mu       sync.Mutex
	start    time.Time
	observer *traceobserver.Observer
	output   *playtrace.Writer
	castID   int
	hasBite  bool
}

func newLiveTrace(model *traceobserver.Model, output *playtrace.Writer, start time.Time) *liveTrace {
	trace := &liveTrace{start: start, output: output, castID: -1}
	if model != nil {
		trace.observer = traceobserver.New(model, math.MaxInt64)
		trace.observer.EnableEvidence(32)
	}
	return trace
}

func (s *liveTrace) cast() func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.castID++
	id := s.castID
	s.hasBite = false
	s.write(s.elapsedUS(), id, playtrace.CastRequested, "")
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.write(s.elapsedUS(), id, playtrace.CastDispatched, traceobserver.OutcomeDryRunDispatched)
	}
}

func (s *liveTrace) bite() func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.castID < 0 || s.hasBite {
		s.castID++
	}
	id := s.castID
	s.hasBite = true
	now := s.elapsedUS()
	s.write(now, id, playtrace.BiteDetected, "")
	s.record(now/1000, id, traceobserver.ActionArrive, 0, "")
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		us := s.elapsedUS()
		s.write(us, id, playtrace.ReelDispatched, traceobserver.OutcomeDryRunDispatched)
		now := us / 1000
		s.record(now, id, traceobserver.ActionAttempt, 1, "")
		s.record(now, id, traceobserver.ActionReply, 1, traceobserver.OutcomeDryRunDispatched)
		s.record(now, id, traceobserver.ActionClose, 1, traceobserver.OutcomeDryRunDispatched)
	}
}

func (s *liveTrace) record(now int64, id int, action string, attempt int, outcome string) {
	if s.observer == nil {
		return
	}
	s.observer.Observe(traceobserver.Record{
		ScenarioID: "app-dry-run", Controller: "app-fsm",
		TMs: now, EventID: id, Action: action, Attempt: attempt, Outcome: outcome,
	})
}

func (s *liveTrace) elapsedUS() int64 { return time.Since(s.start).Microseconds() }

func (s *liveTrace) write(now int64, id int, kind, outcome string) {
	if s.output != nil {
		// Writer reports the first error again when the session closes.
		_ = s.output.Write(playtrace.Event{Type: kind, ElapsedUS: now, EventID: &id, Outcome: outcome})
	}
}

type liveTraceSnapshot struct {
	ElapsedMs    int64                 `json:"elapsed_ms"`
	Result       traceobserver.Result  `json:"result"`
	RecentAlerts []traceobserver.Alert `json:"recent_alerts"`
}

func (s *liveTrace) snapshot() liveTraceSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Since(s.start).Milliseconds()
	s.observer.Advance(now)
	alerts := s.observer.Alerts()
	if alerts == nil {
		alerts = []traceobserver.Alert{}
	}
	return liveTraceSnapshot{now, s.observer.Result(), alerts}
}
