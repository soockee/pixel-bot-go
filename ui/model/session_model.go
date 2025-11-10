package model

import (
	"time"
)

// SessionModel tracks the current session duration and accumulated completed active time.
// Decoupled from UI; presenters poll Values() and update views. Zero value is usable.
type SessionModel struct {
	// active indicates whether capture is currently running.
	active bool
	// captureStart is the timestamp when the current session began.
	captureStart time.Time
	// lastSessionDuration is the duration of the ongoing (if active) or most recent session.
	lastSessionDuration time.Duration
	// accumulated stores the sum of all completed (inactive) session durations.
	accumulated time.Duration
}

// NewSessionModel constructs a new model instance.
func NewSessionModel() *SessionModel { return &SessionModel{} }

// OnTick advances timing given current capturing state at time now. Call periodically (presenter tick).
func (m *SessionModel) OnTick(capturing bool, now time.Time) {
	if m == nil {
		return
	}
	if capturing {
		if !m.active { // transition from off -> on
			m.active = true
			m.captureStart = now
			m.lastSessionDuration = 0
		}
		m.lastSessionDuration = now.Sub(m.captureStart)
	} else if m.active { // transition from on -> off
		m.lastSessionDuration = now.Sub(m.captureStart)
		m.accumulated += m.lastSessionDuration
		m.active = false
	}
}

// Values returns the current session and total durations. Total includes the ongoing session while active.
func (m *SessionModel) Values() (session, total time.Duration) {
	if m == nil {
		return 0, 0
	}
	session = m.lastSessionDuration
	total = m.accumulated
	if m.active {
		total += session
	}
	return
}
