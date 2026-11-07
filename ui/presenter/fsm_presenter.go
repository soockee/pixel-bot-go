package presenter

import (
	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FSMSource provides the fishing FSM methods the presenter requires.
type FSMSource interface {
	Current() fishing.FishingState
}

// StateView sets the state label in the view.
type StateView interface{ SetStateLabel(string) }

// FSMPresenter shows the current FSM state in the view.
type FSMPresenter struct {
	fsm    FSMSource
	view   StateView
	latest fishing.FishingState // last reflected state
}

// NewFSMPresenter returns an FSMPresenter.
func NewFSMPresenter(fsm FSMSource, view StateView) *FSMPresenter {
	return &FSMPresenter{fsm: fsm, view: view, latest: -1}
}

// Tick updates the view when the state has changed.
func (p *FSMPresenter) Tick() {
	if s := p.fsm.Current(); s != p.latest {
		p.latest = s
		p.view.SetStateLabel("State: " + s.String())
	}
}
