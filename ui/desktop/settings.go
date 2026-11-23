package desktop

import (
	"errors"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// Settings are the config values a person can edit in the window.
type Settings struct {
	CastKey                string `json:"castKey"`
	ReelMethod             string `json:"reelMethod"`
	InteractKey            string `json:"interactKey"`
	WaterBuff              string `json:"waterBuff"`
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
		WaterBuff: c.WaterBuff,
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
	check(s.ReelMethod == config.ReelRightClick || s.ReelMethod == config.ReelInteractKey, "Reel method")
	port, err := esp32.PortName(s.ESP32Port)
	if err != nil {
		return c, err
	}
	check(s.WaterBuff == "" || slices.Contains(config.WaterBuffs, s.WaterBuff), "Water walking")
	check(strings.TrimSpace(s.RecordDir) != "", "Recording folder")
	if len(invalid) > 0 {
		return c, errors.New("not saved, invalid: " + strings.Join(invalid, ", "))
	}
	c.CastKey = s.CastKey
	c.ReelMethod, c.InteractKey, c.ESP32Port = s.ReelMethod, s.InteractKey, port
	c.WaterBuff = s.WaterBuff
	c.MaxFailedCasts, c.MaxSessionMinutes = s.MaxFailedCasts, s.MaxSessionMinutes
	c.IdleMoves, c.Reposition = s.IdleMoves, s.Reposition
	c.Schedule = s.Schedule
	c.PlayMinMinutes, c.PlayMaxMinutes = s.PlayMinMinutes, s.PlayMaxMinutes
	c.BreakMinMinutes, c.BreakMaxMinutes = s.BreakMinMinutes, s.BreakMaxMinutes
	c.LogoutKey = s.LogoutKey
	c.RecordDir, c.RecordKeys = strings.TrimSpace(s.RecordDir), s.RecordKeys
	c.CooldownSeconds, c.MaxCastDurationSeconds = s.CooldownSeconds, s.MaxCastDurationSeconds
	// Before Validate, which would fill an empty Cast key with the default.
	// Breaks may be switched on before the Logout key is set; starting a run
	// asks for it (config.CheckRunKeys).
	if err := c.CheckKeys(); err != nil {
		return c, err
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, c.CheckSchedule()
}
