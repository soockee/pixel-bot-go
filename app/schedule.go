package app

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

const (
	screenWait = 90 * time.Second
	loginKey   = 0x0d // Enter on the character selection screen.
)

type schedulePhase uint8

const (
	waitWorld schedulePhase = iota
	play
	sendLogout
	waitCharacters
	takeBreak
	sendLogin
)

type playSchedule struct {
	phase      schedulePhase
	until      time.Time
	nextCheck  time.Time
	confirmed  bool
	logoutOnly bool
	logout     byte
	screen     func(wantWorld bool) bool
	tap        func(vk byte, wantWorld bool) (sent bool, err error)
}

func randomDuration(minMinutes, maxMinutes int) time.Duration {
	minSeconds, maxSeconds := minMinutes*60, maxMinutes*60
	return time.Duration(minSeconds+rand.IntN(maxSeconds-minSeconds+1)) * time.Second
}

func (a *app) beginSchedule() error {
	schedule, err := a.prepareSchedule()
	if err != nil {
		return err
	}
	a.schedule = schedule
	a.recoveryAllowed = true
	a.setSchedulePhase(waitWorld, time.Now().Add(screenWait), "Schedule: focus the logged-in game")
	return nil
}

func (a *app) prepareSchedule() (*playSchedule, error) {
	c := a.container
	cfg := *c.Config
	if err := cfg.CheckSchedule(); err != nil {
		return nil, err
	}
	key, ok := action.ParseVK(cfg.LogoutKey)
	if !ok {
		return nil, errors.New("set the Logout key under Settings, Scheduled breaks")
	}
	for _, other := range []string{cfg.CastKey, cfg.InteractKey, cfg.TargetPlayerKey} {
		if vk, ok := action.ParseVK(other); ok && key == vk {
			return nil, errors.New("the Logout key must differ from the other Go Fish keys")
		}
	}
	if cfg.ESP32Port != "" && !esp32.SupportsKey(key) {
		return nil, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Logout")
	}
	return &playSchedule{logout: key, screen: a.scheduleScreen, tap: a.tapScheduledKey}, nil
}

func (a *app) setSchedulePhase(phase schedulePhase, until time.Time, status string) {
	a.schedule.phase, a.schedule.until = phase, until
	a.schedule.nextCheck = time.Time{}
	a.schedule.confirmed = false
	a.container.View.SetStatus(status)
	a.container.View.SetRunning(true)
}

func (a *app) stopSchedule(status string) {
	a.schedule = nil
	a.recoveryAllowed = false
	c := a.container
	c.CapturePresenter.Disable()
	c.View.SetRunning(false)
	c.View.SetStatus(status)
}

func (a *app) tickSchedule(now time.Time) {
	stopped := a.scheduleStop.Swap(false)
	c := a.container
	failedReposition := !c.Capture.Enabled() && c.FSM.TakeRepositionLogout()
	if stopped {
		a.recoveryAllowed = false
		if a.schedule != nil {
			a.stopSchedule("Schedule stopped")
		}
		return
	}
	if failedReposition && a.recoveryAllowed {
		a.recoveryAllowed = false
		if a.schedule == nil {
			schedule, err := a.prepareSchedule()
			if err != nil {
				c.View.SetStatus(c.FSM.HaltReason() + ". Logout unavailable: " + err.Error())
				return
			}
			a.schedule = schedule
		}
		a.schedule.logoutOnly = true
		a.setSchedulePhase(sendLogout, now.Add(screenWait), c.FSM.HaltReason()+". Preparing to log out")
		return
	}
	s := a.schedule
	if s == nil {
		return
	}
	c.View.SetRunning(true)
	switch s.phase {
	case play:
		if !c.Capture.Enabled() {
			reason := c.FSM.HaltReason()
			if reason == "" {
				reason = "fishing stopped"
			}
			a.stopSchedule("Schedule stopped: " + reason)
		} else if !now.Before(s.until) {
			c.CapturePresenter.Disable()
			a.setSchedulePhase(sendLogout, now.Add(screenWait), "Schedule: preparing to log out")
		}
		return
	case takeBreak:
		if !now.Before(s.until) {
			a.setSchedulePhase(sendLogin, now.Add(screenWait), "Schedule: waiting for character selection to enter world")
		}
		return
	}

	wantWorld := s.phase == waitWorld || s.phase == sendLogout
	if !now.Before(s.until) {
		expected := "character selection"
		if wantWorld {
			expected = "the logged-in game"
		}
		a.stopSchedule("Schedule stopped: could not confirm " + expected + ". Check focus and that Bobber Marker is up to date.")
		return
	}
	if now.Before(s.nextCheck) || c.FSM.Current() != fishing.StateHalt {
		return
	}
	s.nextCheck = now.Add(750 * time.Millisecond)
	matched := s.screen(wantWorld)
	confirmed := matched && s.confirmed
	s.confirmed = matched
	if !confirmed {
		return
	}
	switch s.phase {
	case waitWorld:
		c.CapturePresenter.Enable()
		if !c.Capture.Enabled() {
			a.stopSchedule(c.View.State().Status)
			return
		}
		end := now.Add(randomDuration(c.Config.PlayMinMinutes, c.Config.PlayMaxMinutes))
		a.setSchedulePhase(play, end, "Fishing until "+end.Format("15:04"))
	case sendLogout:
		a.scheduleKey(s.logout, true, waitCharacters, "Schedule: waiting for character selection")
	case waitCharacters:
		if s.logoutOnly {
			a.stopSchedule(c.FSM.HaltReason() + ". Logged out")
			return
		}
		end := now.Add(randomDuration(c.Config.BreakMinMinutes, c.Config.BreakMaxMinutes))
		a.setSchedulePhase(takeBreak, end, "Logged-out break until "+end.Format("15:04"))
	case sendLogin:
		a.scheduleKey(loginKey, false, waitWorld, "Schedule: waiting to enter world")
	}
}

func (a *app) scheduleScreen(wantWorld bool) bool {
	c := a.container
	if a.scheduleStop.Load() || !c.gameFocused() || c.startProblem() != "" {
		return false
	}
	area := c.gameArea()
	if area == nil {
		return false
	}
	signal := fishing.WorldSignalArea.Add(area.Min)
	if !signal.In(*area) {
		return false
	}
	frame, err := c.frames().Grab(signal)
	if err != nil {
		return false
	}
	return fishing.InWorld(frame.Image) == wantWorld
}

func (a *app) scheduleKey(vk byte, wantWorld bool, next schedulePhase, status string) {
	sent, err := a.schedule.tap(vk, wantWorld)
	if err != nil {
		a.stopSchedule("Schedule stopped: " + err.Error())
		return
	}
	if sent {
		a.setSchedulePhase(next, time.Now().Add(screenWait), status)
	} else {
		a.schedule.confirmed = false
	}
}

func (a *app) tapScheduledKey(vk byte, wantWorld bool) (bool, error) {
	c := a.container
	// Open the serial port before rechecking the screen. A handshake can outlast
	// the frame that caused this attempt, and a focus change cancels dispatch.
	var keyboard esp32.Keyboard
	if !c.Config.DryRun && c.Config.ESP32Port != "" {
		port, err := esp32.PortName(c.Config.ESP32Port)
		if err == nil {
			err = keyboard.Open(port)
		}
		if err == nil {
			err = keyboard.PrepareButtons()
		}
		if err != nil {
			return false, err
		}
		defer keyboard.Close()
	}
	if !a.scheduleScreen(wantWorld) || !c.gameFocused() || a.scheduleStop.Load() {
		return false, nil
	}
	switch {
	case c.Config.DryRun:
		a.logger.Info("dry-run scheduled key", "vk", vk)
	case c.Config.ESP32Port != "":
		if !c.gameFocused() || a.scheduleStop.Load() {
			return false, nil
		}
		if err := keyboard.Press(vk, 0); err != nil {
			return false, err
		}
	default:
		release := action.BeginKeyHold(vk)
		if release == nil {
			return false, errors.New("the game key press was blocked")
		}
		time.Sleep(40 * time.Millisecond)
		release()
	}
	return true, nil
}

func (c *AppContainer) gameFocused() bool {
	target := c.gameWindow.Load()
	return target != nil && target.Focused()
}
