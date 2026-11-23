package app

import (
	"errors"
	"image"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/cursormotion"
)

// livePointer is the live fishing.Pointer: it plans each path from where the
// Guard left the OS pointer.
type livePointer struct {
	c     *AppContainer
	guard *action.Guard
	plan  cursormotion.Planner
}

func (p livePointer) Plan(target image.Point, settle bool) ([]fishing.CursorStep, error) {
	start, area, err := p.guard.BeginCursor()
	if err != nil {
		return nil, err
	}
	return cursorSteps(p.plan(start, target, area, settle))
}

func (p livePointer) Reset() { p.guard.ResetCursor() }

func (p livePointer) Approach(center image.Point, box image.Rectangle) (image.Point, bool) {
	area := p.c.gameArea()
	at, ok := p.Pos()
	if area == nil || !ok || !at.In(*area) {
		return center, false
	}
	return cursormotion.ApproachPoint(center, box.Intersect(*area))
}

func (p livePointer) Pos() (image.Point, bool) {
	x, y, ok := action.CursorPos()
	return image.Pt(x, y), ok
}

// dryPointer is a dry run's fishing.Pointer. A dry run moves no pointer, so
// it plans each path from where its last logged move left it, which Moved
// keeps for action.DryRun.
type dryPointer struct {
	c          *AppContainer
	inFront    func() bool
	plan       cursormotion.Planner
	current    image.Point
	positioned bool
}

func (p *dryPointer) Plan(target image.Point, settle bool) ([]fishing.CursorStep, error) {
	if !p.inFront() {
		return nil, fishing.ErrNotInFront
	}
	area := p.c.gameArea()
	if area == nil {
		return nil, errors.New("the game window is not known")
	}
	if !p.positioned || !p.current.In(*area) {
		p.current, p.positioned = area.Min.Add(area.Size().Div(2)), true
	}
	return cursorSteps(p.plan(p.current, target, *area, settle))
}

func (p *dryPointer) Reset() { p.positioned = false }

func (p *dryPointer) Approach(center image.Point, box image.Rectangle) (image.Point, bool) {
	return cursormotion.ApproachPoint(center, box)
}

func (p *dryPointer) Pos() (image.Point, bool) { return p.current, p.positioned }

// Moved records a logged move, and draws it into the trace when one is written.
func (p *dryPointer) Moved(at image.Point, area image.Rectangle) {
	p.current, p.positioned = at, true
	if t := p.c.trace; t != nil && t.output != nil {
		t.cursorWaypoint(at, area)
	}
}

func cursorSteps(path []cursormotion.Step) ([]fishing.CursorStep, error) {
	if len(path) == 0 {
		return nil, errors.New("the planner found no path")
	}
	steps := make([]fishing.CursorStep, len(path))
	for i, step := range path {
		steps[i] = fishing.CursorStep{After: step.After, Point: step.Point}
	}
	return steps, nil
}
