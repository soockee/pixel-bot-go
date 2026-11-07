package config

import (
	"encoding/json"
	"os"
)

// FilePath is the config file, relative to the working directory. The
// misspelling is kept so existing files keep loading.
const FilePath = "pixle_bot_config.json"

// Config holds runtime configuration for detection and app behavior.
// Fields may be loaded from a JSON file and overridden by command-line flags.
type Config struct {
	Debug bool `json:"debug"`
	// Screen picks the monitor the windows open on (1 = primary, 2 = next from
	// the left, ...). 0 keeps the default placement. Overridden by -screen.
	Screen int `json:"screen"`
	// DryRun logs input actions instead of injecting them. Set by the -dry-run
	// flag or PIXELBOT_DRY_RUN=1; never persisted to the config file.
	DryRun bool `json:"-"`
	// AutoWindow preselects the target window title and AutoStart starts capture
	// shortly after launch (headless runs). Set by flags; never persisted. The
	// capture region comes from selection_x/y/w/h in the config file.
	AutoWindow string `json:"-"`
	AutoStart  bool   `json:"-"`
	// Detection parameters
	MinScale  float64 `json:"min_scale"`
	MaxScale  float64 `json:"max_scale"`
	ScaleStep float64 `json:"scale_step"`
	Threshold float64 `json:"threshold"`
	Stride    int     `json:"stride"`
	Refine    bool    `json:"refine"`

	// Selection rectangle persistence (Phase2)
	SelectionX int `json:"selection_x"`
	SelectionY int `json:"selection_y"`
	SelectionW int `json:"selection_w"`
	SelectionH int `json:"selection_h"`

	// CastKey is the key that casts (e.g. "F3" or "R").
	CastKey string `json:"cast_key"`
	// StopKey is the global hotkey that halts the bot. Read at startup.
	StopKey string `json:"stop_key"`
	// MaxFailedCasts halts the bot after this many casts in a row end without a reel.
	MaxFailedCasts int `json:"max_failed_casts"`
	// MaxSessionMinutes halts the bot at the first cast after this long.
	MaxSessionMinutes int `json:"max_session_minutes"`

	// Bite detection configuration (only actively used fields retained).
	ROISizePx int `json:"roi_size_px"` // square ROI side length in pixels
	// MaxCastDurationSeconds defines the maximum expected lifetime of a fishing cast (bobber present).
	// If monitoring exceeds this duration, the target is considered lost and the system returns to searching.
	MaxCastDurationSeconds int `json:"max_cast_duration_seconds"`
	// CooldownSeconds defines how long to wait after reeling before attempting the next cast.
	CooldownSeconds int `json:"cooldown_seconds"`

	// DarkMode persists user preference for dark theme across sessions.
	DarkMode bool `json:"dark_mode"`
}

// DefaultConfig returns a Config populated with standard defaults.
func DefaultConfig() *Config {
	return &Config{
		MinScale:               0.90,
		MaxScale:               1.20,
		ScaleStep:              0.05,
		Threshold:              0.73,
		Stride:                 6,
		Refine:                 true,
		CastKey:                "F3",
		StopKey:                "F10",
		MaxFailedCasts:         5,
		MaxSessionMinutes:      60,
		ROISizePx:              80,
		MaxCastDurationSeconds: 30,
		CooldownSeconds:        8,
		DarkMode:               true,
	}
}

// Validate clamps/normalizes values to safe ranges.
func (c *Config) Validate() error {
	d := DefaultConfig()
	if c.MinScale <= 0 {
		c.MinScale = d.MinScale
	}
	if c.MaxScale < c.MinScale {
		c.MaxScale = c.MinScale + d.MaxScale - d.MinScale
	}
	if c.ScaleStep <= 0 {
		c.ScaleStep = d.ScaleStep
	}
	if span := c.MaxScale - c.MinScale; span > 0 && c.ScaleStep > span {
		c.ScaleStep = span / 4
	}
	if c.Threshold <= 0 || c.Threshold > 1 {
		c.Threshold = d.Threshold
	}
	if c.Stride <= 0 {
		c.Stride = d.Stride
	}
	if c.CastKey == "" {
		c.CastKey = d.CastKey
	}
	if c.StopKey == "" {
		c.StopKey = d.StopKey
	}
	if c.MaxFailedCasts <= 0 {
		c.MaxFailedCasts = d.MaxFailedCasts
	}
	if c.MaxSessionMinutes <= 0 {
		c.MaxSessionMinutes = d.MaxSessionMinutes
	}
	c.ROISizePx = min(max(c.ROISizePx, 32), 256) // keep the ROI modest for performance
	c.MaxCastDurationSeconds = min(max(c.MaxCastDurationSeconds, 5), 180)
	c.CooldownSeconds = min(max(c.CooldownSeconds, 1), 60)
	c.Screen = max(c.Screen, 0)
	return nil
}

// Load attempts to read configuration from the given JSON file path. If the file does not
// exist it returns DefaultConfig(). On JSON error it returns defaults with the error.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	// Files saved before the rename keep their key; a silent F3 could press the wrong one.
	var saved struct {
		CastKey string `json:"cast_key"`
		ReelKey string `json:"reel_key"`
	}
	if err := json.Unmarshal(data, &saved); err == nil && saved.CastKey == "" && saved.ReelKey != "" {
		cfg.CastKey = saved.ReelKey
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
