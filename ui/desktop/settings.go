package desktop

import (
	"errors"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// Settings are the config values a person can edit in the window.
type Settings struct {
	CastKey                string `json:"castKey"`
	ReelMethod             string `json:"reelMethod"`
	InteractKey            string `json:"interactKey"`
	ESP32Port              string `json:"esp32Port"`
	IdleMoves              bool   `json:"idleMoves"`
	Reposition             bool   `json:"reposition"`
	MaxFailedCasts         int    `json:"maxFailedCasts"`
	MaxSessionMinutes      int    `json:"maxSessionMinutes"`
	Schedule               bool   `json:"schedule"`
	PlayMinMinutes         int    `json:"playMinMinutes"`
	PlayMaxMinutes         int    `json:"playMaxMinutes"`
	BreakMinMinutes        int    `json:"breakMinMinutes"`
	BreakMaxMinutes        int    `json:"breakMaxMinutes"`
	LogoutKey              string `json:"logoutKey"`
	RecordDir              string `json:"recordDir"`
	RecordKeys             bool   `json:"recordKeys"`
	CooldownSeconds        int    `json:"cooldownSeconds"`
	MaxCastDurationSeconds int    `json:"maxCastDurationSeconds"`
}

func settingsOf(c *config.Config) Settings {
	return Settings{
		CastKey:    c.CastKey,
		ReelMethod: c.ReelMethod, InteractKey: c.InteractKey, ESP32Port: c.ESP32Port,
		IdleMoves: c.IdleMoves, Reposition: c.Reposition,
		MaxFailedCasts: c.MaxFailedCasts, MaxSessionMinutes: c.MaxSessionMinutes,
		Schedule: c.Schedule, PlayMinMinutes: c.PlayMinMinutes, PlayMaxMinutes: c.PlayMaxMinutes,
		BreakMinMinutes: c.BreakMinMinutes, BreakMaxMinutes: c.BreakMaxMinutes,
		LogoutKey: c.LogoutKey,
		RecordDir: c.RecordDir, RecordKeys: c.RecordKeys,
		CooldownSeconds: c.CooldownSeconds, MaxCastDurationSeconds: c.MaxCastDurationSeconds,
	}
}

// apply returns a copy of c with s applied and clamped to safe ranges by
// config.Validate, or an error naming every invalid field.
func (s Settings) apply(c config.Config) (config.Config, error) {
	var invalid []string
	check := func(ok bool, label string) {
		if !ok {
			invalid = append(invalid, label)
		}
	}
	castVK, castOK := action.ParseVK(s.CastKey)
	check(castOK && esp32.SupportsKey(castVK), "Cast key")
	check(s.ReelMethod == config.ReelRightClick || s.ReelMethod == config.ReelInteractKey, "Reel method")
	interactVK, interactOK := action.ParseVK(s.InteractKey)
	if s.ReelMethod == config.ReelInteractKey {
		if !interactOK {
			return c, errors.New("set the Interact key before enabling this reel method")
		}
		if sameKey(s.InteractKey, s.CastKey) {
			return c, errors.New("the Interact key must differ from the Cast key")
		}
	} else if strings.TrimSpace(s.InteractKey) != "" {
		check(interactOK, "Interact key")
	}
	port, err := esp32.PortName(s.ESP32Port)
	if err != nil {
		return c, err
	}
	if s.ReelMethod == config.ReelInteractKey && !esp32.SupportsKey(interactVK) {
		return c, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Interact")
	}
	check(strings.TrimSpace(s.RecordDir) != "", "Recording folder")
	if len(invalid) > 0 {
		return c, errors.New("not saved, invalid: " + strings.Join(invalid, ", "))
	}
	// Breaks may be switched on before the Logout key is set; starting a run
	// asks for it (config.CheckSchedule).
	if s.LogoutKey != "" {
		vk, ok := action.ParseVK(s.LogoutKey)
		if !ok || sameKey(s.LogoutKey, s.CastKey) ||
			s.ReelMethod == config.ReelInteractKey && sameKey(s.LogoutKey, s.InteractKey) {
			return c, errors.New("the Logout key must differ from the Cast and Interact keys")
		}
		if !esp32.SupportsKey(vk) {
			return c, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Logout")
		}
	}
	c.CastKey = s.CastKey
	c.ReelMethod, c.InteractKey, c.ESP32Port = s.ReelMethod, s.InteractKey, port
	c.MaxFailedCasts, c.MaxSessionMinutes = s.MaxFailedCasts, s.MaxSessionMinutes
	c.IdleMoves, c.Reposition = s.IdleMoves, s.Reposition
	c.Schedule = s.Schedule
	c.PlayMinMinutes, c.PlayMaxMinutes = s.PlayMinMinutes, s.PlayMaxMinutes
	c.BreakMinMinutes, c.BreakMaxMinutes = s.BreakMinMinutes, s.BreakMaxMinutes
	c.LogoutKey = s.LogoutKey
	c.RecordDir, c.RecordKeys = strings.TrimSpace(s.RecordDir), s.RecordKeys
	c.CooldownSeconds, c.MaxCastDurationSeconds = s.CooldownSeconds, s.MaxCastDurationSeconds
	if err := c.Validate(); err != nil {
		return c, err
	}
	if err := fishing.CheckActivityKeys(c, action.ParseVK); err != nil {
		return c, err
	}
	return c, c.CheckSchedule()
}

func sameKey(a, b string) bool {
	av, aOK := action.ParseVK(a)
	bv, bOK := action.ParseVK(b)
	return aOK && bOK && av == bv
}
