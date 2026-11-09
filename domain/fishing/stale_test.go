package fishing

import (
	"image"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

// A cast that got no bite is followed at once by the next cast, while the old
// bobber is still in the water. The game fades it out, and its marker stays up
// for a moment. The new cast must not lock onto it.
func TestNewCastDoesNotLockTheBobberItReplaces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine, fsm, frames, _ := newSceneEngine(t, recordingActions(&actionLog{}))
		defer fsm.Close()
		startEngine(t, engine)
		defer engine.Stop()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()
		time.Sleep(500 * time.Millisecond)
		wantState(t, fsm, StateMonitoring) // the first bobber

		fsm.ForceCast() // the next cast; the old marker is still on screen
		time.Sleep(1500 * time.Millisecond)
		wantState(t, fsm, StateSearching)

		frames.set(fakegame.PhaseEmpty) // the old bobber has faded
		time.Sleep(500 * time.Millisecond)
		frames.set(fakegame.PhaseBobber) // the new one lands in the same place
		time.Sleep(500 * time.Millisecond)
		wantState(t, fsm, StateMonitoring)
	})
}

// A marker that never goes away is the new bobber: the old one has faded long
// before staleFor ends, so the cast must not be lost to waiting for a gap.
func TestMarkerThatStaysAfterACastIsTheNewBobber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine, fsm, _, _ := newSceneEngine(t, recordingActions(&actionLog{}))
		defer fsm.Close()
		startEngine(t, engine)
		defer engine.Stop()
		fsm.EventAwaitFocus()
		fsm.EventFocusAcquired()
		time.Sleep(500 * time.Millisecond)
		wantState(t, fsm, StateMonitoring)

		fsm.ForceCast()
		time.Sleep(staleFor + 500*time.Millisecond)
		wantState(t, fsm, StateMonitoring)
	})
}

func TestMarkerIsStale(t *testing.T) {
	last := image.Rect(100, 100, 196, 196)
	cases := []struct {
		name  string
		box   image.Rectangle
		since time.Duration
		gone  bool // the marker has been missing since the cast
		want  bool
	}{
		{"same place right after the cast", last, 200 * time.Millisecond, false, true},
		{"same place, slightly shifted", last.Add(image.Pt(6, -4)), time.Second, false, true},
		{"same place, but it vanished and came back", last, time.Second, true, false},
		{"same place, the fade is over", last, staleFor, false, false},
		{"another place", last.Add(image.Pt(300, 0)), 200 * time.Millisecond, false, false},
		{"only touching the old box", last.Add(image.Pt(90, 0)), 200 * time.Millisecond, false, false},
	}
	for _, tc := range cases {
		if got := markerIsStale(tc.box, last, tc.since, tc.gone); got != tc.want {
			t.Errorf("%s: markerIsStale = %v, want %v", tc.name, got, tc.want)
		}
	}
	if markerIsStale(last, image.Rectangle{}, 0, false) {
		t.Error("a marker is never stale when there was no earlier bobber")
	}
}
