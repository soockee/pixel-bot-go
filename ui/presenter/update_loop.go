package presenter

import "time"

// Loop drives the presenters from the UI tick.
type Loop struct {
	session *SessionPresenter
	fsm     *FSMPresenter
	capture *CapturePresenter
	preview *PreviewPresenter
}

// NewLoop returns a Loop over the given presenters.
func NewLoop(session *SessionPresenter, fsm *FSMPresenter, capture *CapturePresenter, preview *PreviewPresenter) *Loop {
	return &Loop{session: session, fsm: fsm, capture: capture, preview: preview}
}

// Tick runs one UI update.
func (l *Loop) Tick() {
	now := time.Now()
	l.fsm.Tick()
	l.capture.Tick()
	l.session.Tick(now)
	l.preview.Tick(now)
}
