package config

import (
	"encoding/json"
	"os"
)

// Config holds runtime configuration for detection and app behavior.
// Fields may be loaded from a JSON file and overridden by command-line flags.
type Config struct {
	Debug bool `json:"debug"`
	// Detection parameters
	MinScale       float64 `json:"min_scale"`
	MaxScale       float64 `json:"max_scale"`
	ScaleStep      float64 `json:"scale_step"`
	Threshold      float64 `json:"threshold"`
	Stride         int     `json:"stride"`
	Refine         bool    `json:"refine"`
	UseRGB         bool    `json:"use_rgb"`
	StopOnScore    float64 `json:"stop_on_score"`
	ReturnBestEven bool    `json:"return_best_even"`

	// Selection rectangle persistence (Phase2)
	SelectionX int `json:"selection_x"`
	SelectionY int `json:"selection_y"`
	SelectionW int `json:"selection_w"`
	SelectionH int `json:"selection_h"`

	// Reel key configuration (e.g. "F3" or "R")
	ReelKey string `json:"reel_key"`

	// Bite detection configuration (only actively used fields retained).
	ROISizePx int `json:"roi_size_px"` // square ROI side length in pixels
	// MaxCastDurationSeconds defines the maximum expected lifetime of a fishing cast (bobber present).
	// If monitoring exceeds this duration, the target is considered lost and the system returns to searching.
	MaxCastDurationSeconds int `json:"max_cast_duration_seconds"`
	// CooldownSeconds defines how long to wait after reeling before attempting the next cast.
	CooldownSeconds int `json:"cooldown_seconds"`
}

// DefaultConfig returns a Config populated with standard defaults.
func DefaultConfig() *Config {
	return &Config{
		Debug:                  false,
		MinScale:               0.90,
		MaxScale:               1.90,
		ScaleStep:              0.1,
		Threshold:              0.80,
		Stride:                 4,
		Refine:                 true,
		UseRGB:                 false,
		StopOnScore:            0.93,
		ReturnBestEven:         true,
		SelectionX:             0,
		SelectionY:             0,
		SelectionW:             0,
		SelectionH:             0,
		ReelKey:                "F3",
		ROISizePx:              80,
		MaxCastDurationSeconds: 16,
		CooldownSeconds:        7,
	}
}

// Validate clamps/normalizes values to safe ranges.
func (c *Config) Validate() error {
	if c.MinScale <= 0 {
		c.MinScale = 0.60
	}
	if c.MaxScale <= 0 || c.MaxScale < c.MinScale {
		c.MaxScale = c.MinScale + 0.80
	}
	if c.ScaleStep <= 0 {
		c.ScaleStep = 0.05
	}
	if c.ScaleStep > (c.MaxScale - c.MinScale) {
		c.ScaleStep = (c.MaxScale - c.MinScale) / 4
	}
	if c.Threshold <= 0 || c.Threshold > 1 {
		c.Threshold = 0.80
	}
	if c.Stride <= 0 {
		c.Stride = 4
	}
	if c.StopOnScore < 0 || c.StopOnScore > 1 {
		c.StopOnScore = 0.95
	}
	if c.ReelKey == "" {
		c.ReelKey = "F3"
	}
	// Bite detection validation & sane clamps
	if c.ROISizePx < 32 {
		c.ROISizePx = 32
	}
	if c.ROISizePx > 256 { // keep ROI modest for performance
		c.ROISizePx = 256
	}

	if c.MaxCastDurationSeconds < 5 { // extremely short casts are unlikely; enforce reasonable floor
		c.MaxCastDurationSeconds = 5
	}
	if c.MaxCastDurationSeconds > 180 { // safety upper bound (3 minutes) though typical is ~30s
		c.MaxCastDurationSeconds = 180
	}

	// Cooldown seconds sanity (allow zero -> default minimal, clamp upper bound for safety)
	if c.CooldownSeconds <= 0 {
		c.CooldownSeconds = 1
	}
	if c.CooldownSeconds > 60 { // more than a minute likely unnecessary
		c.CooldownSeconds = 60
	}

	return nil
}

// Load attempts to read configuration from the given JSON file path. If the file does not
// exist it returns DefaultConfig(). On JSON error it returns defaults with the error.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(cfg); err != nil {
		return cfg, err
	}
	_ = cfg.Validate()
	return cfg, nil
}

// Save writes the configuration to the given path in JSON format.
func (c *Config) Save(path string) error {
	_ = c.Validate()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}
