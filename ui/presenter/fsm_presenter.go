package presenter

import (
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FSMSource exposes the domain fishing FSM methods presenter needs.
type FSMSource interface {
	Tick(time.Time)
	Current() fishing.FishingState
}

// StateView updates state label.
type StateView interface{ SetStateLabel(string) }

// FSMPresenter consumes FSM ticks & pending state changes and updates the view.
type FSMPresenter struct {
	eng     FSMSource
	view    StateView
	latest  fishing.FishingState // last reflected state
	pending []fishing.FishingState
}

func NewFSMPresenter(eng FSMSource, view StateView) *FSMPresenter {
	return &FSMPresenter{eng: eng, view: view}
}

// OnState queues a state transitioned from FSM listener.
func (p *FSMPresenter) OnState(s fishing.FishingState) {
	if p == nil {
		return
	}
	p.pending = append(p.pending, s)
}

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
	p.eng.Tick(now)
}
