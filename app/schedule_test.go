package app

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/ui/desktop"
	"github.com/soocke/pixel-bot-go/ui/model"
	"github.com/soocke/pixel-bot-go/ui/presenter"
)

type schedulePipeline struct{}

func (schedulePipeline) Start() error { return nil }
func (schedulePipeline) Stop()        {}

func TestScheduledBreakWaitsForBothGameScreens(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Schedule = true
		cfg.PlayMinMinutes, cfg.PlayMaxMinutes = 2, 2
		cfg.BreakMinMinutes, cfg.BreakMaxMinutes = 3, 3
		fsm := fishing.NewFSM(nil, cfg, fishing.ActionCallbacks{})
		defer fsm.Close()
		c := &AppContainer{Config: cfg, FSM: fsm, Capture: &model.CaptureModel{}, View: desktop.NewView(desktop.NewFrames(), func(desktop.State) {})}
		c.CapturePresenter = presenter.NewCapturePresenter(c.Capture, schedulePipeline{}, fsm, c.View)
		a := &app{container: c}
		world := true
		var keys []byte
		a.schedule = &playSchedule{
			logout: 'L',
			screen: func(wantWorld bool) bool { return world == wantWorld },
			tap: func(vk byte, _ bool) (bool, error) {
				keys = append(keys, vk)
				return true, nil
			},
		}
		a.setSchedulePhase(waitWorld, time.Now().Add(screenWait), "")
		a.tickSchedule(time.Now())
		synctest.Wait()
		if !c.Capture.Enabled() || a.schedule.phase != play {
			t.Fatal("scheduled play did not start")
		}
		time.Sleep(2 * time.Minute)
		a.tickSchedule(time.Now())
		synctest.Wait()
		a.tickSchedule(time.Now())
		if c.Capture.Enabled() || a.schedule.phase != waitCharacters || !slices.Equal(keys, []byte{'L'}) {
			t.Fatalf("logout did not stop capture and dispatch once: %v", keys)
		}
		time.Sleep(time.Second)
		a.tickSchedule(time.Now())
		if a.schedule.phase != waitCharacters {
			t.Fatal("started break before character selection")
		}
		world = false
		time.Sleep(time.Second)
		a.tickSchedule(time.Now())
		if a.schedule.phase != takeBreak || a.schedule.until.Sub(time.Now()) != 3*time.Minute {
			t.Fatal("break timer did not start at character selection")
		}
		time.Sleep(3 * time.Minute)
		world = true
		a.tickSchedule(time.Now())
		a.tickSchedule(time.Now())
		if !slices.Equal(keys, []byte{'L'}) || a.schedule.phase != sendLogin {
			t.Fatal("sent Enter without confirming character selection")
		}
		world = false
		time.Sleep(time.Second)
		a.tickSchedule(time.Now())
		if !slices.Equal(keys, []byte{'L', loginKey}) || a.schedule.phase != waitWorld || c.Capture.Enabled() {
			t.Fatalf("login did not wait for world: %v", keys)
		}
		world = true
		a.tickSchedule(time.Now())
		synctest.Wait()
		if a.schedule.phase != play || !c.Capture.Enabled() {
			t.Fatal("fishing did not resume after the world appeared")
		}
		c.CapturePresenter.Disable()
		synctest.Wait()
		a.setSchedulePhase(takeBreak, time.Now(), "")
		a.scheduleStop.Store(true)
		a.tickSchedule(time.Now())
		if a.schedule != nil || c.Capture.Enabled() || c.View.State().Running || !slices.Equal(keys, []byte{'L', loginKey}) {
			t.Fatal("Stop did not cancel the whole schedule")
		}
	})
}
