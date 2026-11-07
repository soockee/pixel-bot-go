package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/soocke/pixel-bot-go/internal/traceobserver"
)

const (
	actionArrive     = traceobserver.ActionArrive
	actionAttempt    = traceobserver.ActionAttempt
	actionReply      = traceobserver.ActionReply
	actionClose      = traceobserver.ActionClose
	outcomeOK        = traceobserver.OutcomeOK
	outcomeFail      = traceobserver.OutcomeFail
	outcomeTimeout   = traceobserver.OutcomeTimeout
	outcomeLate      = traceobserver.OutcomeLate
	outcomeCompleted = traceobserver.OutcomeCompleted
	outcomeTimedOut  = traceobserver.OutcomeTimedOut
	outcomeMissed    = traceobserver.OutcomeMissed
)

type Record = traceobserver.Record

// Same-time order: a reply lands before the deadline, so a success exactly at
// the deadline counts, and the deadline closes the event before a wake can use it.
const (
	stepReply = iota
	stepDeadline
	stepArrive
	stepWake
)

type step struct {
	at      int64
	kind    int
	seq     int // insertion order settles remaining ties deterministically
	id      int
	attempt int
	outcome string
}

func compareSteps(a, b step) int {
	return cmp.Or(cmp.Compare(a.at, b.at), cmp.Compare(a.kind, b.kind), cmp.Compare(a.seq, b.seq))
}

type simulation struct {
	queue []step
	seq   int
}

func (s *simulation) push(st step) {
	st.seq = s.seq
	s.seq++
	i, _ := slices.BinarySearchFunc(s.queue, st, compareSteps)
	s.queue = slices.Insert(s.queue, i, st)
}

// simulate runs one controller over one scenario on a virtual clock and returns
// the trace. One action runs at a time, and busy time is the only cost of an action.
func simulate(sc Scenario, c Controller, name string) ([]Record, error) {
	return simulateObserved(sc, c, name, nil)
}

// The observer runs at the point of emission, before the next controller decision.
func simulateObserved(sc Scenario, c Controller, name string, observe func(Record)) ([]Record, error) {
	env := sc.Environment
	arrivals := sc.arrivals()
	sim := &simulation{}
	for id, at := range arrivals {
		sim.push(step{at: at, kind: stepArrive, id: id})
		sim.push(step{at: at + env.WindowMs, kind: stepDeadline, id: id})
	}

	open := make([]bool, len(arrivals))
	attempts := make([]int, len(arrivals))
	var trace []Record
	emit := func(t int64, id int, action string, attempt int, outcome string) {
		record := Record{
			ScenarioID: sc.ID, Seed: sc.Seed, Controller: name, TMs: t,
			EventID: id, Action: action, Attempt: attempt, Outcome: outcome,
		}
		trace = append(trace, record)
		if observe != nil {
			observe(record)
		}
	}

	now := int64(0)
	busy := false
	wakeAt := int64(-1)
	for len(sim.queue) > 0 {
		st := sim.queue[0]
		sim.queue = sim.queue[1:]
		if st.at < now {
			return nil, fmt.Errorf("%s/%s: clock went back from %d to %d", sc.ID, name, now, st.at)
		}
		now = st.at

		switch st.kind {
		case stepArrive:
			open[st.id] = true
			emit(now, st.id, actionArrive, 0, "")
			c.OnArrive(now, st.id)
		case stepDeadline:
			if open[st.id] {
				open[st.id] = false
				outcome := outcomeMissed
				if attempts[st.id] > 0 {
					outcome = outcomeTimedOut
				}
				emit(now, st.id, actionClose, attempts[st.id], outcome)
				c.OnClosed(st.id)
			}
		case stepReply:
			busy = false
			outcome := st.outcome
			if !open[st.id] {
				outcome = outcomeLate
			}
			emit(now, st.id, actionReply, st.attempt, outcome)
			if outcome == outcomeOK {
				open[st.id] = false
				emit(now, st.id, actionClose, attempts[st.id], outcomeCompleted)
			}
			c.OnResult(now, st.id, outcome == outcomeOK)
		case stepWake:
			wakeAt = -1
		}

		if busy {
			continue
		}
		id, at, ok := c.Next(now)
		switch {
		case !ok:
		case at < now:
			return nil, fmt.Errorf("%s/%s: action scheduled in the past (%d < %d)", sc.ID, name, at, now)
		case at > now:
			if at != wakeAt {
				wakeAt = at
				sim.push(step{at: at, kind: stepWake})
			}
		default:
			if id < 0 || id >= len(open) || !open[id] {
				return nil, fmt.Errorf("%s/%s: action on closed event %d at %d", sc.ID, name, id, now)
			}
			attempts[id]++
			emit(now, id, actionAttempt, attempts[id], "")
			busy = true
			reply := step{at: now + env.ServiceMs, kind: stepReply, id: id, attempt: attempts[id], outcome: outcomeOK}
			switch sc.faultAt(now) {
			case faultFail:
				reply.outcome = outcomeFail
			case faultHang:
				reply.at, reply.outcome = now+env.AttemptTimeoutMs, outcomeTimeout
			}
			sim.push(reply)
		}
	}
	return trace, nil
}
