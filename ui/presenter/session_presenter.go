package presenter

import (
	"time"

	"github.com/soocke/pixel-bot-go/ui/model"
)

// CaptureEnabledModel supplies capture enabled state.
type CaptureEnabledModel interface{ Enabled() bool }

// SessionView receives formatted session durations.
type SessionView interface {
	SetSession(sessionSeconds, totalSeconds int)
}

// SessionPresenter formats session & total durations from model to view.
type SessionPresenter struct {
	sess *model.SessionModel
	cap  CaptureEnabledModel
	view SessionView
}

func NewSessionPresenter(sess *model.SessionModel, cap CaptureEnabledModel, view SessionView) *SessionPresenter {
	return &SessionPresenter{sess: sess, cap: cap, view: view}
}

func (p *SessionPresenter) Tick(now time.Time) {
	if p == nil || p.sess == nil || p.cap == nil || p.view == nil {
		return
	}
	p.sess.OnTick(p.cap.Enabled(), now)
	s, t := p.sess.Values()
	p.view.SetSession(int(s.Seconds()), int(t.Seconds()))
}
