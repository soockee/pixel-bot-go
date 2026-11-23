package presenter

import (
	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FSMSource provides the fishing FSM methods the presenter requires.
type FSMSource interface {
	Current() fishing.FishingState
	PausedByMouse() bool
}

// MousePauseLabel is the state label while the person's mouse paused the run;
// it is the focus state, waiting for the mouse to rest rather than for focus.
const MousePauseLabel = "mouse"

// StateView sets the state label in the view.
type StateView interface{ SetStateLabel(string) }

// FSMPresenter shows the current FSM state in the view.
type FSMPresenter struct {
	fsm    FSMSource
	view   StateView
	latest string // last reflected label
}

// NewFSMPresenter returns an FSMPresenter.
func NewFSMPresenter(fsm FSMSource, view StateView) *FSMPresenter {
	return &FSMPresenter{fsm: fsm, view: view}
}

// Tick updates the view when the state has changed.
func (p *FSMPresenter) Tick() {
	label := p.fsm.Current().String()
	if p.fsm.PausedByMouse() {
		label = MousePauseLabel
	}
	if label != p.latest {
		p.latest = label
		p.view.SetStateLabel(label)
	}
}
