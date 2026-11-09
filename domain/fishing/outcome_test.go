package fishing

import (
	"image"
	"image/color"
	"testing"
)

// signal draws Bobber Marker's outcome blocks with counter seq and outcome o.
func signal(seq uint8, o Outcome) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 24, 6))
	blocks := []color.RGBA{{255, 255, 0, 255}, {0, 255, 255, 255}, {255, 255, 0, 255}, {seq, uint8(o), 77, 255}}
	for i, c := range blocks {
		for y := range 6 {
			for x := range 6 {
				img.SetRGBA(i*6+x, y, c)
			}
		}
	}
	return img
}

func TestReadOutcome(t *testing.T) {
	for _, o := range []Outcome{OutcomeCaught, OutcomeGotAway, OutcomeNotHooked} {
		seq, got, ok := readOutcome(signal(7, o))
		if !ok || seq != 7 || got != o {
			t.Errorf("%s: read %d %s %v", o, seq, got, ok)
		}
	}
	if _, _, ok := readOutcome(image.NewRGBA(image.Rect(0, 0, 24, 6))); ok {
		t.Error("read an outcome from a black area")
	}
	if _, _, ok := readOutcome(signal(7, 100)); ok {
		t.Error("read an unknown outcome")
	}
}

// Each change of the counter is one outcome; the first reading of a run is
// what an earlier run left.
func TestOutcomesCountEachChangeOnce(t *testing.T) {
	var w outcomes
	steps := []struct {
		seq  uint8
		o    Outcome
		ok   bool
		want Outcome
	}{
		{3, OutcomeCaught, true, OutcomeNone},         // left from an earlier run
		{3, OutcomeCaught, true, OutcomeNone},         // the same
		{4, OutcomeNotHooked, true, OutcomeNotHooked}, // new
		{0, OutcomeNone, false, OutcomeNone},          // hidden for a moment
		{4, OutcomeNotHooked, true, OutcomeNone},      // still the same
		{5, OutcomeGotAway, true, OutcomeGotAway},
	}
	for i, s := range steps {
		if got := w.see(s.seq, s.o, s.ok); got != s.want {
			t.Errorf("step %d: %s, want %s", i, got, s.want)
		}
	}

	var fresh outcomes
	fresh.see(0, OutcomeNone, false) // no blocks yet when the run starts
	if got := fresh.see(1, OutcomeCaught, true); got != OutcomeCaught {
		t.Errorf("the first outcome after a start without blocks: %s", got)
	}
}
