package model

import (
	"time"
)

// SessionModel tracks the current session duration and the accumulated active time.
// It is decoupled from the UI; presenters should poll Values() and update views.
// The zero value is ready to use.
type SessionModel struct {
	active              bool
	captureStart        time.Time
	lastSessionDuration time.Duration
	accumulated         time.Duration
}

// NewSessionModel returns a pointer to a ready-to-use SessionModel.
func NewSessionModel() *SessionModel { return &SessionModel{} }

// OnTick updates the model using the current capture state and timestamp.
// Call periodically (for example, from a presenter tick).
func (m *SessionModel) OnTick(capturing bool, now time.Time) {
	if m == nil {
		return
	}
	if capturing {
		if !m.active { // transition off -> on
			m.active = true
			m.captureStart = now
			m.lastSessionDuration = 0
		}
		m.lastSessionDuration = now.Sub(m.captureStart)
	} else if m.active { // transition on -> off
		m.lastSessionDuration = now.Sub(m.captureStart)
		m.accumulated += m.lastSessionDuration
		m.active = false
	}
}

// Values returns the current session duration and the total accumulated duration.
// The total includes the ongoing session when active.
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
