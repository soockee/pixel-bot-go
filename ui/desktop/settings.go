package desktop

import (
	"errors"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/internal/esp32"
)

// Settings are the config values a person can edit in the window.
type Settings struct {
	MinScale               float64 `json:"minScale"`
	MaxScale               float64 `json:"maxScale"`
	ScaleStep              float64 `json:"scaleStep"`
	Threshold              float64 `json:"threshold"`
	Stride                 int     `json:"stride"`
	Refine                 bool    `json:"refine"`
	CastKey                string  `json:"castKey"`
	ReelMethod             string  `json:"reelMethod"`
	InteractKey            string  `json:"interactKey"`
	ESP32Port              string  `json:"esp32Port"`
	Background             bool    `json:"background"`
	IdleMoves              bool    `json:"idleMoves"`
	Reposition             bool    `json:"reposition"`
	TargetPlayerKey        string  `json:"targetPlayerKey"`
	MaxFailedCasts         int     `json:"maxFailedCasts"`
	MaxSessionMinutes      int     `json:"maxSessionMinutes"`
	Schedule               bool    `json:"schedule"`
	PlayMinMinutes         int     `json:"playMinMinutes"`
	PlayMaxMinutes         int     `json:"playMaxMinutes"`
	BreakMinMinutes        int     `json:"breakMinMinutes"`
	BreakMaxMinutes        int     `json:"breakMaxMinutes"`
	LogoutKey              string  `json:"logoutKey"`
	Locator                string  `json:"locator"`
	BiteDetector           string  `json:"biteDetector"`
	RecordDir              string  `json:"recordDir"`
	RecordKeys             bool    `json:"recordKeys"`
	ROISizePx              int     `json:"roiSizePx"`
	CooldownSeconds        int     `json:"cooldownSeconds"`
	MaxCastDurationSeconds int     `json:"maxCastDurationSeconds"`
}

// SettingsForm is the form content: the values and the choices for the
// technique fields.
type SettingsForm struct {
	Values        Settings `json:"values"`
	Locators      []string `json:"locators"`
	BiteDetectors []string `json:"biteDetectors"`
}

func settingsOf(c *config.Config) Settings {
	return Settings{
		MinScale: c.MinScale, MaxScale: c.MaxScale, ScaleStep: c.ScaleStep, Threshold: c.Threshold,
		Stride: c.Stride, Refine: c.Refine, CastKey: c.CastKey,
		ReelMethod: c.ReelMethod, InteractKey: c.InteractKey, ESP32Port: c.ESP32Port, Background: c.Background,
		IdleMoves: c.IdleMoves, Reposition: c.Reposition, TargetPlayerKey: c.TargetPlayerKey,
		MaxFailedCasts: c.MaxFailedCasts, MaxSessionMinutes: c.MaxSessionMinutes,
		Schedule: c.Schedule, PlayMinMinutes: c.PlayMinMinutes, PlayMaxMinutes: c.PlayMaxMinutes,
		BreakMinMinutes: c.BreakMinMinutes, BreakMaxMinutes: c.BreakMaxMinutes,
		LogoutKey: c.LogoutKey,
		Locator:   c.Locator, BiteDetector: c.BiteDetector, RecordDir: c.RecordDir, RecordKeys: c.RecordKeys,
		ROISizePx: c.ROISizePx, CooldownSeconds: c.CooldownSeconds, MaxCastDurationSeconds: c.MaxCastDurationSeconds,
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
	check(castOK, "Cast key")
	check(s.ReelMethod == config.ReelRightClick || s.ReelMethod == config.ReelInteractKey, "Reel method")
	interactVK, interactOK := action.ParseVK(s.InteractKey)
	if s.ReelMethod == config.ReelInteractKey {
		if !interactOK {
			return c, errors.New("set the Interact key before enabling this reel method")
		}
		if sameKey(s.InteractKey, s.CastKey) {
			return c, errors.New("the Interact key must differ from the Cast key")
		}
		if s.Locator != "marker" {
			return c, errors.New("Interact key requires the marker locator")
		}
	} else if strings.TrimSpace(s.InteractKey) != "" {
		check(interactOK, "Interact key")
	}
	port, err := esp32.PortName(s.ESP32Port)
	if err != nil {
		return c, err
	}
	if port != "" {
		if !esp32.SupportsKey(castVK) {
			return c, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Cast")
		}
		if s.ReelMethod == config.ReelInteractKey && !esp32.SupportsKey(interactVK) {
			return c, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Interact")
		}
	}
	// config.Validate turns Background off without the Interact reel or with the ESP32.
	if s.Background && s.ReelMethod == config.ReelInteractKey && port == "" &&
		(isSideButton(s.CastKey) || isSideButton(s.InteractKey)) {
		return c, errors.New("fishing in the background needs keyboard keys for Cast and Interact, not mouse buttons")
	}
	check(slices.Contains(detect.LocatorNames(), s.Locator), "Locator")
	check(slices.Contains(detect.BiteDetectorNames(), s.BiteDetector), "Bite detector")
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
		if port != "" && !esp32.SupportsKey(vk) {
			return c, errors.New("ESP32 requires a letter, digit, F1–F12, Mouse 4 or Mouse 5 for Logout")
		}
	}
	c.MinScale, c.MaxScale, c.ScaleStep, c.Threshold = s.MinScale, s.MaxScale, s.ScaleStep, s.Threshold
	c.Stride, c.Refine, c.CastKey = s.Stride, s.Refine, s.CastKey
	c.ReelMethod, c.InteractKey, c.ESP32Port, c.Background = s.ReelMethod, s.InteractKey, port, s.Background
	c.MaxFailedCasts, c.MaxSessionMinutes = s.MaxFailedCasts, s.MaxSessionMinutes
	c.IdleMoves, c.Reposition, c.TargetPlayerKey = s.IdleMoves, s.Reposition, s.TargetPlayerKey
	c.Schedule = s.Schedule
	c.PlayMinMinutes, c.PlayMaxMinutes = s.PlayMinMinutes, s.PlayMaxMinutes
	c.BreakMinMinutes, c.BreakMaxMinutes = s.BreakMinMinutes, s.BreakMaxMinutes
	c.LogoutKey = s.LogoutKey
	c.Locator, c.BiteDetector = s.Locator, s.BiteDetector
	c.RecordDir, c.RecordKeys = strings.TrimSpace(s.RecordDir), s.RecordKeys
	c.ROISizePx, c.CooldownSeconds, c.MaxCastDurationSeconds = s.ROISizePx, s.CooldownSeconds, s.MaxCastDurationSeconds
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

// isSideButton reports whether key names a mouse side button, which cannot be
// sent to a window in the background.
func isSideButton(key string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(key)), "MOUSE")
}
