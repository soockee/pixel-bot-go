package fishing

import (
	"image"
	"time"
)

// Outcome is how a cast ended, as Bobber Marker reports it.
type Outcome uint8

// The outcomes, by the green Bobber Marker gives the signal's last block.
const (
	OutcomeNone      Outcome = 0
	OutcomeCaught    Outcome = 64  // the fishing loot opened
	OutcomeGotAway   Outcome = 128 // "Your fish got away!": reeled on a bite, the skill was too low
	OutcomeNotHooked Outcome = 192 // "No fish are hooked": reeled with nothing on the hook, a false bite
)

func (o Outcome) String() string {
	switch o {
	case OutcomeCaught:
		return "caught"
	case OutcomeGotAway:
		return "got_away"
	case OutcomeNotHooked:
		return "not_hooked"
	}
	return "none"
}

// OutcomeSignalArea is where Bobber Marker shows how the last cast ended, in
// the game window's client area: under the in-world blocks.
var OutcomeSignalArea = image.Rect(8, 24, 32, 30)

// readOutcome reads a capture of OutcomeSignalArea: yellow, cyan and yellow
// blocks, then a block whose red counts outcomes and whose green is the
// outcome. It reports false when the blocks are not there.
func readOutcome(img *image.RGBA) (seq uint8, o Outcome, ok bool) {
	if img == nil || img.Bounds().Dx() < 24 || img.Bounds().Dy() < 6 {
		return 0, OutcomeNone, false
	}
	near := func(a, b uint8) bool { return max(a, b)-min(a, b) <= 24 }
	origin := img.Bounds().Min
	for i, rgb := range [3][3]uint8{{255, 255, 0}, {0, 255, 255}, {255, 255, 0}} {
		c := img.RGBAAt(origin.X+3+i*6, origin.Y+3)
		if !near(c.R, rgb[0]) || !near(c.G, rgb[1]) || !near(c.B, rgb[2]) {
			return 0, OutcomeNone, false
		}
	}
	data := img.RGBAAt(origin.X+21, origin.Y+3)
	if !near(data.B, 77) || data.R == 0 {
		return 0, OutcomeNone, false
	}
	for _, known := range []Outcome{OutcomeCaught, OutcomeGotAway, OutcomeNotHooked} {
		if near(data.G, uint8(known)) {
			return data.R, known, true
		}
	}
	return 0, OutcomeNone, false
}

// outcomes turns successive readings into new outcomes: the first reading of
// a run is what an earlier run left, and each later change of the counter is
// one new outcome.
type outcomes struct {
	started bool
	last    uint8
}

func (w *outcomes) see(seq uint8, o Outcome, ok bool) Outcome {
	if !ok {
		seq = 0
	}
	first := !w.started
	w.started = true
	if first || seq == 0 || seq == w.last {
		if seq != 0 {
			w.last = seq
		}
		return OutcomeNone
	}
	w.last = seq
	return o
}

// pollOutcome reads Bobber Marker's outcome blocks four times a second and
// counts each new outcome in the run's stats.
func (r *run) pollOutcome(window image.Rectangle) {
	now := time.Now()
	if now.Before(r.outcomePoll) {
		return
	}
	r.outcomePoll = now.Add(250 * time.Millisecond)
	e := r.engine
	if e.fsm.actions.CanAct != nil && !e.fsm.actions.CanAct() {
		return
	}
	area := OutcomeSignalArea.Add(window.Min)
	if !area.In(window) {
		return
	}
	frame, err := e.frames.Grab(area)
	if err != nil {
		return
	}
	seq, o, ok := readOutcome(frame.Image)
	if o = r.outcomes.see(seq, o, ok); o != OutcomeNone {
		e.fsm.countOutcome(o)
		e.logger.Info("fishing outcome", "outcome", o.String())
	}
}

// countOutcome counts how a cast ended. Safe from any goroutine.
func (f *FishingFSM) countOutcome(o Outcome) {
	switch o {
	case OutcomeCaught:
		f.stats.caught.Add(1)
	case OutcomeGotAway:
		f.stats.gotAway.Add(1)
	case OutcomeNotHooked:
		f.stats.notHooked.Add(1)
	}
}
