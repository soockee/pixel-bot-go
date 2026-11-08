package desktop

import (
	"errors"
	"slices"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/detect"
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
	StopKey                string  `json:"stopKey"`
	MaxFailedCasts         int     `json:"maxFailedCasts"`
	MaxSessionMinutes      int     `json:"maxSessionMinutes"`
	Locator                string  `json:"locator"`
	BiteDetector           string  `json:"biteDetector"`
	RecordDir              string  `json:"recordDir"`
	RecordKeys             bool    `json:"recordKeys"`
	ShareRecordings        bool    `json:"shareRecordings"`
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
		Stride: c.Stride, Refine: c.Refine, CastKey: c.CastKey, StopKey: c.StopKey,
		MaxFailedCasts: c.MaxFailedCasts, MaxSessionMinutes: c.MaxSessionMinutes,
		Locator: c.Locator, BiteDetector: c.BiteDetector, RecordDir: c.RecordDir, RecordKeys: c.RecordKeys, ShareRecordings: c.ShareRecordings,
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
	_, castOK := action.ParseVK(s.CastKey)
	check(castOK, "Cast key")
	_, stopOK := action.ParseVK(s.StopKey)
	check(stopOK, "Stop key")
	if sameKey(s.CastKey, s.StopKey) {
		return c, errors.New("the Cast and Stop keys must be different")
	}
	check(slices.Contains(detect.LocatorNames(), s.Locator), "Locator")
	check(slices.Contains(detect.BiteDetectorNames(), s.BiteDetector), "Bite detector")
	check(strings.TrimSpace(s.RecordDir) != "", "Recording folder")
	if len(invalid) > 0 {
		return c, errors.New("not saved, invalid: " + strings.Join(invalid, ", "))
	}
	c.MinScale, c.MaxScale, c.ScaleStep, c.Threshold = s.MinScale, s.MaxScale, s.ScaleStep, s.Threshold
	c.Stride, c.Refine, c.CastKey, c.StopKey = s.Stride, s.Refine, s.CastKey, s.StopKey
	c.MaxFailedCasts, c.MaxSessionMinutes = s.MaxFailedCasts, s.MaxSessionMinutes
	c.Locator, c.BiteDetector = s.Locator, s.BiteDetector
	c.RecordDir, c.RecordKeys, c.ShareRecordings = strings.TrimSpace(s.RecordDir), s.RecordKeys, s.ShareRecordings
	c.ROISizePx, c.CooldownSeconds, c.MaxCastDurationSeconds = s.ROISizePx, s.CooldownSeconds, s.MaxCastDurationSeconds
	return c, c.Validate()
}

func sameKey(a, b string) bool {
	av, aOK := action.ParseVK(a)
	bv, bOK := action.ParseVK(b)
	return aOK && bOK && av == bv
}
