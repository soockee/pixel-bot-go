package presenter

import "time"

// Loop aggregates feature presenters and drives periodic updates.
//
// It calls Tick/ProcessFrame on the sub-presenters and invokes a
// scheduler callback. The zero value is usable (methods are nil-safe).
type Loop struct {
	Session  *SessionPresenter
	FSM      *FSMPresenter
	Detect   *DetectionPresenter
	Schedule func()
}

func NewLoop(sess *SessionPresenter, fsm *FSMPresenter, detect *DetectionPresenter, schedule func()) *Loop {
	return &Loop{Session: sess, FSM: fsm, Detect: detect, Schedule: schedule}
}

func (l *Loop) Tick() {
	if l == nil {
		return
	}
	now := time.Now()
	// Drive FSM presenter so it can flush pending state changes to the view.
	if l.FSM != nil {
		l.FSM.Tick(now)
	}
	if l.Session != nil {
		l.Session.Tick(now)
	}
	if l.Detect != nil {
		l.Detect.ProcessFrame()
	}
	if l.Schedule != nil {
		l.Schedule()
	}
}
