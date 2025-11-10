package presenter

import (
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FSMSource provides the fishing FSM methods the presenter requires.
type FSMSource interface {
	Current() fishing.FishingState
}

// StateView sets the state label in the view.
type StateView interface{ SetStateLabel(string) }

// FSMPresenter receives FSM ticks and pending state changes, and updates the view.
type FSMPresenter struct {
	eng     FSMSource
	view    StateView
	latest  fishing.FishingState // last reflected state
	pending []fishing.FishingState
}

func NewFSMPresenter(eng FSMSource, view StateView) *FSMPresenter {
	return &FSMPresenter{eng: eng, view: view}
}

// OnState queues a transitioned state from the FSM listener.
//
// The latest queued state will be reflected on the next Tick.
func (p *FSMPresenter) OnState(s fishing.FishingState) {
	if p == nil {
		return
	}
	p.pending = append(p.pending, s)
}

// Tick processes queued states and updates the view with the most recent state.
// It clears the pending queue after processing.
func (p *FSMPresenter) Tick(now time.Time) {
	if p == nil || p.eng == nil || p.view == nil {
		return
	}
	if len(p.pending) > 0 {
		last := p.pending[len(p.pending)-1]
		p.pending = p.pending[:0]
		if last != p.latest {
			p.latest = last
			p.view.SetStateLabel("State: " + last.String())
		}
	}
}
