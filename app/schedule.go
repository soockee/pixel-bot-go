package app

import (
	"errors"
	"fmt"
	"image"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/cloud"
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
	phase     schedulePhase
	until     time.Time
	nextCheck time.Time
	confirmed bool
	logout    byte
	world     *image.RGBA
	character *image.RGBA
	screen    func(wantWorld bool) bool
	tap       func(vk byte, wantWorld bool) (sent bool, err error)
}

func randomDuration(minMinutes, maxMinutes int) time.Duration {
	minSeconds, maxSeconds := minMinutes*60, maxMinutes*60
	return time.Duration(minSeconds+rand.IntN(maxSeconds-minSeconds+1)) * time.Second
}

func (a *app) beginSchedule() error {
	c := a.container
	cfg := *c.Config
	if err := cfg.CheckSchedule(); err != nil {
		return err
	}
	key, ok := action.ParseVK(cfg.LogoutKey)
	if !ok {
		return errors.New("set a valid Logout key")
	}
	for _, other := range []string{cfg.CastKey, cfg.InteractKey, cfg.StopKey, cfg.IdleKey, cfg.TargetPlayerKey, c.View.State().StopKey} {
		if vk, ok := action.ParseVK(other); ok && key == vk {
			return errors.New("the Logout key must differ from the other Go Fish keys")
		}
	}
	if cfg.ESP32Port != "" && !esp32.SupportsKey(key) {
		return errors.New("ESP32 requires a keyboard binding for Logout")
	}
	world, err := readSchedulePNG(cfg.InWorldPNG)
	if err != nil {
		return fmt.Errorf("in-game image: %w", err)
	}
	character, err := readSchedulePNG(cfg.CharacterPNG)
	if err != nil {
		return fmt.Errorf("character screen image: %w", err)
	}
	if containsScreenPatch(world, character) || containsScreenPatch(character, world) {
		return errors.New("choose distinct controls for the two screen images")
	}
	a.schedule = &playSchedule{
		logout: key, world: world, character: character,
		screen: a.scheduleScreen, tap: a.tapScheduledKey,
	}
	a.setSchedulePhase(waitWorld, time.Now().Add(screenWait), "Schedule: focus the logged-in game")
	return nil
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
	c := a.container
	c.CapturePresenter.Disable()
	c.View.SetRunning(false)
	c.View.SetStatus(status)
}

func (a *app) tickSchedule(now time.Time) {
	stopped := a.scheduleStop.Swap(false)
	s := a.schedule
	if s == nil {
		return
	}
	c := a.container
	if stopped || a.cloud != nil && !c.Config.DryRun && !a.can(cloud.PermFish) {
		a.stopSchedule("Schedule stopped")
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
		a.stopSchedule("Schedule stopped: could not confirm " + expected + ". Check focus and the saved screen image.")
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
	frame, err := c.frames().Grab(*area)
	if err != nil {
		return false
	}
	s := a.schedule
	world := containsScreenPatch(frame.Image, s.world)
	character := containsScreenPatch(frame.Image, s.character)
	return world == wantWorld && character != wantWorld
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
		usage, ok := esp32.ForegroundUsage(vk)
		if !ok || !c.gameFocused() || a.scheduleStop.Load() {
			return false, nil
		}
		if err := keyboard.Tap(usage); err != nil {
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
	title, err := action.ForegroundWindowTitle()
	return err == nil && title != "" && strings.EqualFold(title, strings.TrimSpace(c.Capture.Window()))
}
