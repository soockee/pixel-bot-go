package fishing

import (
	"fmt"
	"image"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// CheckActivityKeys rejects optional keys that could perform another Go Fish action.
func CheckActivityKeys(cfg config.Config, parse func(string) (byte, bool)) error {
	if cfg.IdleKey == "" && cfg.TargetPlayerKey == "" && cfg.RepositionKey == "" {
		return nil
	}
	if parse == nil {
		return fmt.Errorf("optional game keys cannot be resolved")
	}
	used := make(map[byte]string)
	for _, binding := range []struct {
		key, label string
		optional   bool
	}{
		{cfg.CastKey, "Cast", false},
		{cfg.InteractKey, "Interact", false},
		{cfg.StopKey, "Stop", false},
		{cfg.LogoutKey, "Logout", false},
		{cfg.IdleKey, "Between-catch action", true},
		{repositionBinding(cfg.RepositionKey), "Reposition key", true},
		{cfg.TargetPlayerKey, "Target passing player", true},
	} {
		if binding.key == "" {
			continue
		}
		vk, ok := parse(binding.key)
		if binding.optional {
			if !ok || !esp32.SupportsKey(vk) {
				return fmt.Errorf("%s requires a letter, digit or F1–F12", binding.label)
			}
			if other, exists := used[vk]; exists {
				return fmt.Errorf("%s needs a different key from %s", binding.label, other)
			}
		}
		if ok {
			used[vk] = binding.label
		}
	}
	if cfg.RepositionKey == config.RepositionAuto {
		for _, key := range "WASD" {
			if vk, ok := parse(string(key)); ok {
				if other, exists := used[vk]; exists {
					return fmt.Errorf("automatic reposition uses %s, which is assigned to %s", string(key), other)
				}
			}
		}
	}
	return nil
}

func repositionBinding(key string) string {
	if key == config.RepositionAuto {
		return ""
	}
	return key
}

func (f *FishingFSM) activity(key, name string) bool {
	if key == "" || f.actions.ParseVK == nil {
		return false
	}
	vk, ok := f.actions.ParseVK(key)
	if !ok {
		return false
	}
	if f.actions.CanAct != nil && !f.actions.CanAct() {
		f.transition(StateWaitingFocus)
		return false
	}
	if f.hardwareKeyboard() {
		if !f.tapKeyboard(vk, 0) {
			f.halt()
			return false
		}
	} else if f.actions.HoldInteract != nil {
		release := f.actions.HoldInteract(vk)
		if release == nil {
			return false
		}
		time.Sleep(interactHold)
		release()
	} else if !f.actions.PressKey(vk) {
		return false
	}
	f.logger.Info(name, "key", key)
	return true
}

func (r *run) pollPlayer(window image.Rectangle) {
	now := time.Now()
	if now.Before(r.playerPoll) {
		return
	}
	r.playerPoll = now.Add(250 * time.Millisecond)
	e := r.engine
	sample := func() uint8 {
		if e.fsm.actions.CanAct != nil && !e.fsm.actions.CanAct() {
			return 0
		}
		current, err := e.frames.Window()
		if err != nil || current != window {
			return 0
		}
		area := image.Rect(8, 8, 38, 14).Add(window.Min)
		if !area.In(window) {
			return 0
		}
		frame, err := e.frames.Grab(area)
		if err != nil {
			return 0
		}
		return passingPlayerSignal(frame.Image)
	}
	player := sample()
	if player != 0 && player == r.playerSeen && player != r.playerSent {
		r.playerSent = player
		e.fsm.send(evtPassingPlayer{confirm: func() bool { return sample() == player }})
	}
	r.playerSeen = player
}

// The addon's opaque cyan/yellow/magenta/blue blocks identify an eligible
// soft target. Sample their centers to avoid pixel rounding at the edges.
func passingPlayerSignal(img *image.RGBA) uint8 {
	if img == nil || img.Bounds().Dx() < 30 || img.Bounds().Dy() < 6 {
		return 0
	}
	origin := img.Bounds().Min
	for i, rgb := range [4][3]uint8{{0, 255, 255}, {255, 255, 0}, {255, 0, 255}, {0, 0, 255}} {
		x, y := origin.X+3+i*6, origin.Y+3
		c := img.RGBAAt(x, y)
		if c.R != rgb[0] || c.G != rgb[1] || c.B != rgb[2] {
			return 0
		}
	}
	token := img.RGBAAt(origin.X+27, origin.Y+3)
	if token.G != 211 || token.B != 77 {
		return 0
	}
	return token.R
}
