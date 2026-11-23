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
)

func TestWanderPathDriftsAShortSmoothWay(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	start, area := image.Pt(500, 400), image.Rect(0, 0, 1000, 800)
	for range 200 {
		path := wanderPath(start, area, rng.IntN)
		if len(path) < 10 || len(path) > 22 {
			t.Fatalf("%d points", len(path))
		}
		end := path[len(path)-1].Sub(start)
		if d := end.X*end.X + end.Y*end.Y; d < 38*38 || d > 162*162 {
			t.Fatalf("ends %v from the start", end)
		}
		wantSmoothInside(t, start, path, area, 45)
		if !path[len(path)-1].In(idleBounds(area)) {
			t.Fatalf("ends at %v, outside %v", path[len(path)-1], idleBounds(area))
		}
	}
}

func TestSwirlPathCirclesTheWindow(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	area := image.Rect(-1920, 100, -1920+1600, 100+900) // a window on a monitor left of the primary
	for range 200 {
		start := image.Pt(area.Min.X+rng.IntN(area.Dx()), area.Min.Y+rng.IntN(area.Dy()))
		path := swirlPath(start, area, rng.IntN)
		wantSmoothInside(t, start, path, area, 80)
		span := image.Rectangle{Min: path[0], Max: path[0]}
		for _, p := range path {
			span = span.Union(image.Rectangle{Min: p, Max: p.Add(image.Pt(1, 1))})
		}
		if span.Dx() < area.Dx()/3 || span.Dy() < area.Dy()/3 {
			t.Fatalf("swirl spans %v of a %v window", span.Size(), area.Size())
		}
	}
}

// wantSmoothInside fails when path leaves in or jumps more than step pixels.
func wantSmoothInside(t *testing.T, start image.Point, path []image.Point, in image.Rectangle, step int) {
	t.Helper()
	prev := start
	for _, p := range path {
		if !p.In(in) {
			t.Fatalf("%v is outside %v", p, in)
		}
		if d := p.Sub(prev); d.X*d.X+d.Y*d.Y > step*step {
			t.Fatalf("jumps %v in one step", d)
		}
		prev = p
	}
}

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

// idleFSM is an FSM with IdleMoves on, whose pointer rests in a 1000x800
// window and whose moves are recorded with the state they were made in.
func idleFSM(roll func(int) int) (m *FishingFSM, moves func() []stateMove) {
	var mu sync.Mutex
	var made []stateMove
	deps := recordingDeps(&actionLog{})
	deps.Roll = roll
	at, area := image.Pt(500, 400), image.Rect(0, 0, 1000, 800)
	pointerOf(deps).pos = func() (image.Point, image.Rectangle, bool) { return at, area, true }
	cfg := testConfig()
	cfg.IdleMoves = true
	inputOf(deps).move = func(p image.Point) error {
		mu.Lock()
		defer mu.Unlock()
		made = append(made, stateMove{m.state, p}) // MoveTo runs on the FSM loop, which owns state
		at = p
		return nil
	}
	m = NewFSM(nil, cfg, deps)
	return m, func() []stateMove { mu.Lock(); defer mu.Unlock(); return slices.Clone(made) }
}

type stateMove struct {
	state FishingState
	at    image.Point
}

// catchOne reels a bobber at (10,10) and waits for the next cast.
func catchOne(t *testing.T, m *FishingFSM) {
	t.Helper()
	m.EventTargetAcquiredAt(10, 10)
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

func TestIdleMovesRunOnlyBetweenCatches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, moves := idleFSM(rand.New(rand.NewPCG(5, 6)).IntN)
		defer m.Close()
		m.EventAwaitFocus()
		m.EventFocusAcquired()
		wantState(t, m, StateSearching)
		for range 12 {
			catchOne(t, m)
		}
		idle, swirls := 0, 0
		var span image.Rectangle // what the current cooldown's moves covered
		wide := func() {
			if span.Dx() >= 300 {
				swirls++
			}
			span = image.Rectangle{}
		}
		for _, mv := range moves() {
			switch mv.state {
			case StateMonitoring, StateReeling:
				if mv.at != image.Pt(10, 10) {
					t.Fatalf("a move to %v while %v; only the bobber may be aimed at then", mv.at, mv.state)
				}
				wide()
			case StateCooldown:
				idle++
				span = span.Union(image.Rectangle{Min: mv.at, Max: mv.at.Add(image.Pt(1, 1))})
			default:
				t.Fatalf("a move to %v while %v", mv.at, mv.state)
			}
		}
		wide()
		if idle < 12*10 || swirls == 0 {
			t.Fatalf("%d idle moves and %d swirls in 12 catches, want a path every catch and a swirl", idle, swirls)
		}
	})
}

func TestTransitionsEndAnIdleSwirl(t *testing.T) {
	for name, interrupt := range map[string]func(m *FishingFSM){
		"cast":       (*FishingFSM).ForceCast,
		"focus lost": (*FishingFSM).EventFocusLost,
		"stop":       (*FishingFSM).EventHalt,
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Every roll 0 makes the path a swirl.
				m, moves := idleFSM(func(int) int { return 0 })
				defer m.Close()
				startMonitoring(t, m)
				m.EventFishBite()
				time.Sleep(reelClickDelay + time.Second + 500*time.Millisecond)
				wantState(t, m, StateCooldown)
				before := len(moves())
				if before < 15 {
					t.Fatalf("%d moves half a second into the swirl", before)
				}
				interrupt(m)
				synctest.Wait()
				after := len(moves())
				time.Sleep(10 * time.Second)
				if got := len(moves()); got != after || after > before+1 {
					t.Fatalf("moves %d before, %d at and %d after the %s; the swirl went on", before, after, got, name)
				}
				if name != "cast" {
					return
				}
				catchOne(t, m) // the next cooldown starts a new path from the bobber
				for _, mv := range moves()[after:] {
					if mv.state == StateCooldown {
						if d := mv.at.Sub(image.Pt(10, 10)); d.X*d.X+d.Y*d.Y > 80*80 {
							t.Fatalf("the next cooldown went on with the old swirl at %v", mv.at)
						}
						break
					}
				}
			})
		})
	}
}
