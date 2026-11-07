package presenter

import (
	"time"

	"github.com/soocke/pixel-bot-go/ui/model"
)

// CaptureEnabledModel reports whether capture is enabled.
type CaptureEnabledModel interface{ Enabled() bool }

// SessionView displays formatted session and total durations.
type SessionView interface {
	SetSession(session, total time.Duration)
}

// SessionPresenter formats session and total durations from the model to the view.
type SessionPresenter struct {
	sess    *model.SessionModel
	capture CaptureEnabledModel
	view    SessionView
}

// NewSessionPresenter returns a new SessionPresenter.
func NewSessionPresenter(sess *model.SessionModel, capture CaptureEnabledModel, view SessionView) *SessionPresenter {
	return &SessionPresenter{sess: sess, capture: capture, view: view}
}

// Tick advances the session model and pushes its values to the view.
func (p *SessionPresenter) Tick(now time.Time) {
	p.sess.OnTick(p.capture.Enabled(), now)
	s, t := p.sess.Values()
	p.view.SetSession(s, t)
}
