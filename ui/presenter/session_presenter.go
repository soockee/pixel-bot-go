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
	sess *model.SessionModel
	cap  CaptureEnabledModel
	view SessionView
}

// NewSessionPresenter returns a new SessionPresenter.
func NewSessionPresenter(sess *model.SessionModel, cap CaptureEnabledModel, view SessionView) *SessionPresenter {
	return &SessionPresenter{sess: sess, cap: cap, view: view}
}

// Tick updates the presenter: advance the session model and push values to the view.
func (p *SessionPresenter) Tick(now time.Time) {
	if p == nil || p.sess == nil || p.cap == nil || p.view == nil {
		return
	}
	p.sess.OnTick(p.cap.Enabled(), now)
	s, t := p.sess.Values()
	p.view.SetSession(s, t)
}
