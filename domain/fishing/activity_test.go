package fishing

import (
	"image"
	"image/color"
	"math/rand/v2"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/internal/fakegame"
)

func TestPassingPlayerSignalDoesNotInterruptFishing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &actionLog{}
		actions := recordingActions(log)
		actions.ParseVK = func(key string) (byte, bool) {
			if key == "K" {
				return 'K', true
			}
			return 0, false
		}
		engine, fsm, frames, _ := newSceneEngine(t, actions)
		defer fsm.Close()
		engine.cfg.TargetPlayerKey = "K"
		startMonitoring(t, fsm)
		window, err := frames.Window()
		if err != nil {
			t.Fatal(err)
		}
		// Keep the fake bobber in place while a player enters and leaves the
		// soft-target slot. The engine samples only the small signal strip.
		var sequence uint8 = 17
		render := frames.render
		frames.render = func(phase fakegame.Phase) *image.RGBA {
			img := render(phase)
			for i, pixel := range []color.RGBA{
				{0, 255, 255, 255}, {255, 255, 0, 255}, {255, 0, 255, 255}, {0, 0, 255, 255}, {sequence, 211, 77, 255},
			} {
				img.SetRGBA(11+i*6, 11, pixel)
			}
			return img
		}
		r := &run{engine: engine}
		r.pollPlayer(window)
		wantActions(t, log, "move")
		time.Sleep(250 * time.Millisecond)
		r.pollPlayer(window)
		wantActions(t, log, "move", "press")
		time.Sleep(250 * time.Millisecond)
		r.pollPlayer(window)
		wantActions(t, log, "move", "press")
		sequence++
		time.Sleep(250 * time.Millisecond)
		r.pollPlayer(window)
		wantActions(t, log, "move", "press")
		time.Sleep(250 * time.Millisecond)
		r.pollPlayer(window)
		wantActions(t, log, "move", "press", "press")
		wantState(t, fsm, StateMonitoring)
		x, y, ok := fsm.TargetCoordinates()
		if !ok || x != 10 || y != 10 {
			t.Fatalf("player action changed fishing coordinates to (%d,%d), valid=%v", x, y, ok)
		}
		fsm.EventFishBite()
		time.Sleep(reelClickDelay)
		wantState(t, fsm, StateCooldown)
		wantActions(t, log, "move", "press", "press", "move", "click")
	})
}

func TestWanderPathDriftsAShortSmoothWay(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	start := image.Pt(500, 400)
	for range 200 {
		path := wanderPath(start, rng.IntN)
		if len(path) < 10 || len(path) > 16 {
			t.Fatalf("%d points", len(path))
		}
		end := path[len(path)-1].Sub(start)
		if d := end.X*end.X + end.Y*end.Y; d < 38*38 || d > 162*162 {
			t.Fatalf("ends %v from the start", end)
		}
		prev := start
		for _, p := range path {
			if step := p.Sub(prev); step.X*step.X+step.Y*step.Y > 45*45 {
				t.Fatalf("jumps %v in one step", step)
			}
			prev = p
		}
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
