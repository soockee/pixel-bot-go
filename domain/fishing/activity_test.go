package fishing

import (
	"image"
	"image/color"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

func TestInWorldReadsTheAddonBlocks(t *testing.T) {
	img := image.NewRGBA(WorldSignalArea.Sub(WorldSignalArea.Min))
	if InWorld(img) {
		t.Fatal("a black capture counted as in world")
	}
	for i, c := range []color.RGBA{{250, 4, 251, 255}, {3, 252, 0, 255}, {255, 0, 255, 255}} {
		for x := i * 6; x < i*6+6; x++ {
			for y := range 6 {
				img.SetRGBA(x, y, c)
			}
		}
	}
	if !InWorld(img) {
		t.Fatal("the addon's blocks did not count as in world")
	}
	img.SetRGBA(9, 3, color.RGBA{255, 0, 255, 255}) // the middle block turns magenta
	if InWorld(img) {
		t.Fatal("a wrong middle block counted as in world")
	}
}

// glanceFSM is an FSM with IdleMoves and Reposition on, whose pointer starts
// in the middle of a 1000x800 window. Its moves, and its taps other than the
// Cast key, are recorded with the state they were made in, and a click fails
// the test unless the pointer is on the bobber at (10,10).
func glanceFSM(t *testing.T, roll func(int) int) (m *FishingFSM, moves func() []stateMove, steps func() []FishingState) {
	var mu sync.Mutex
	var made []stateMove
	var tapped []FishingState
	deps := recordingDeps(&actionLog{})
	deps.Roll = roll
	at, area := image.Pt(500, 400), image.Rect(0, 0, 1000, 800)
	pointerOf(deps).pos = func() (image.Point, image.Rectangle, bool) { return at, area, true }
	cfg := testConfig()
	cfg.IdleMoves, cfg.Reposition = true, true
	cast, _ := config.KeyCode(cfg.CastKey)
	in := inputOf(deps)
	in.tap = func(vk byte, _ time.Duration) error {
		if vk != cast {
			mu.Lock()
			tapped = append(tapped, m.state) // input runs on the FSM loop, which owns state
			mu.Unlock()
		}
		return nil
	}
	in.move = func(p image.Point) error {
		mu.Lock()
		defer mu.Unlock()
		made = append(made, stateMove{m.state, p})
		at = p
		return nil
	}
	in.click = func() error {
		if at != image.Pt(10, 10) {
			t.Errorf("reeled with the pointer at %v, not on the bobber", at)
		}
		return nil
	}
	m = NewFSM(nil, cfg, deps)
	return m, func() []stateMove { mu.Lock(); defer mu.Unlock(); return slices.Clone(made) },
		func() []FishingState { mu.Lock(); defer mu.Unlock(); return slices.Clone(tapped) }
}

type stateMove struct {
	state FishingState
	at    image.Point
}

// catchAfter watches a bobber at (10,10) for watch, reels it and waits for the next cast.
func catchAfter(t *testing.T, m *FishingFSM, watch time.Duration) {
	t.Helper()
	m.EventTargetAcquiredAt(10, 10)
	wantState(t, m, StateMonitoring)
	time.Sleep(watch)
	wantState(t, m, StateMonitoring)
	m.EventFishBite()
	for range 400 {
		time.Sleep(25 * time.Millisecond)
		if m.Current() == StateSearching {
			return
		}
	}
	t.Fatalf("state = %v after a catch, want searching", m.Current())
}

func TestGlancesRunOnlyWhileWatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, moves, steps := glanceFSM(t, rand.New(rand.NewPCG(5, 6)).IntN)
		defer m.Close()
		m.EventAwaitFocus()
		m.EventFocusAcquired()
		wantState(t, m, StateSearching)
		for i := range 30 {
			catchAfter(t, m, time.Duration(i%10)*time.Second) // bites at every point of a glance
		}
		repositions := steps()
		if len(repositions) == 0 {
			t.Fatal("no change of spot in 30 catches")
		}
		for _, s := range repositions {
			if s != StateCooldown {
				t.Fatalf("a change of spot while %v", s)
			}
		}
		glances := 0
		for _, mv := range moves() {
			switch {
			case mv.state != StateMonitoring && mv.state != StateReeling:
				t.Fatalf("a move to %v while %v", mv.at, mv.state)
			case mv.state == StateReeling && mv.at != image.Pt(10, 10):
				t.Fatalf("a reel aimed at %v", mv.at)
			case mv.at != image.Pt(10, 10):
				glances++
				if !mv.at.In(image.Rect(80, 80, 920, 720)) {
					t.Fatalf("a glance to %v, near the window's edge", mv.at)
				}
			}
		}
		if glances < 20 {
			t.Fatalf("%d glances in 30 catches", glances)
		}
	})
}

func TestTransitionsEndTheGlances(t *testing.T) {
	for name, interrupt := range map[string]func(m *FishingFSM){
		"bite":       (*FishingFSM).EventFishBite,
		"cast":       (*FishingFSM).ForceCast,
		"focus lost": (*FishingFSM).EventFocusLost,
		"stop":       (*FishingFSM).EventHalt,
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Every roll 0: the first glance goes to (80,80) 1.5 s in.
				m, moves, _ := glanceFSM(t, func(int) int { return 0 })
				defer m.Close()
				startMonitoring(t, m)
				time.Sleep(1600 * time.Millisecond)
				if got := moves(); got[len(got)-1].at != image.Pt(80, 80) {
					t.Fatalf("moves %v, want a glance to (80,80)", got)
				}
				interrupt(m)
				time.Sleep(10 * time.Second)
				for _, mv := range moves() {
					if mv.state == StateMonitoring {
						continue
					}
					if mv.state != StateReeling || mv.at != image.Pt(10, 10) {
						t.Fatalf("a move to %v while %v after the %s", mv.at, mv.state, name)
					}
				}
			})
		})
	}
}
