package fishing

import (
	"image"
	"image/color"
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
