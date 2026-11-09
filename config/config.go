package config

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
)

// CheckSchedule validates the play and break ranges. The Logout key is
// checked when a run starts, so breaks can be switched on before it is set.
func (c Config) CheckSchedule() error {
	if c.Schedule && (c.PlayMinMinutes < 1 || c.PlayMaxMinutes < c.PlayMinMinutes || c.PlayMaxMinutes > 240 ||
		c.BreakMinMinutes < 1 || c.BreakMaxMinutes < c.BreakMinMinutes || c.BreakMaxMinutes > 240) {
		return errors.New("set ordered play and break ranges from 1 to 240 minutes")
	}
	return nil
}

// FilePath is the config file, relative to the working directory. The
// misspelling is kept so existing files keep loading.
const FilePath = "pixle_bot_config.json"

// LogPath is the JSON log, relative to the working directory, truncated at
// each start.
const LogPath = "pixel_bot_logs.json"

const (
	ReelRightClick  = "right_click"  // Reel by clicking the bobber.
	ReelInteractKey = "interact_key" // Reel with the game's native Interact binding.
)

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
	// app captures that window's client area.
	AutoWindow string `json:"-"`
	AutoStart  bool   `json:"-"`
	// Detection parameters
	MinScale  float64 `json:"min_scale"`
	MaxScale  float64 `json:"max_scale"`
	ScaleStep float64 `json:"scale_step"`
	Threshold float64 `json:"threshold"`
	Stride    int     `json:"stride"`
	Refine    bool    `json:"refine"`

	// CastKey is the key that casts (e.g. "F3" or "R").
	CastKey string `json:"cast_key"`
	// ReelMethod selects how the app interacts with a marked bobber.
	ReelMethod string `json:"reel_method"`
	// InteractKey matches the game's native Interact binding when selected.
	InteractKey string `json:"interact_key,omitempty"`
	// ESP32Port enables the serial BLE keyboard backend when set.
	ESP32Port string `json:"esp32_port,omitempty"`
	// IdleMoves now and then wanders the cursor or nudges the camera between catches.
	IdleMoves bool `json:"idle_moves,omitempty"`
	// Reposition steps the character with WASD, or turns the camera with a
	// mouse drag, every 45 to 55 catches, before a verification cast.
	Reposition bool `json:"reposition,omitempty"`
	// TargetPlayerKey selects the nearby friendly player reported by the addon.
	TargetPlayerKey string `json:"target_player_key,omitempty"`
	// MaxFailedCasts halts the bot after this many casts in a row end without a reel.
	MaxFailedCasts int `json:"max_failed_casts"`
	// MaxSessionMinutes halts the bot at the first cast after this long.
	MaxSessionMinutes int `json:"max_session_minutes"`
	// Schedule enables alternating play and logged-out breaks after Start.
	Schedule        bool   `json:"schedule,omitempty"`
	PlayMinMinutes  int    `json:"play_min_minutes"`
	PlayMaxMinutes  int    `json:"play_max_minutes"`
	BreakMinMinutes int    `json:"break_min_minutes"`
	BreakMaxMinutes int    `json:"break_max_minutes"`
	LogoutKey       string `json:"logout_key,omitempty"`
	// Locator and BiteDetector name the techniques to use. The registered names
	// are in domain/detect/registry.go.
	Locator      string `json:"locator"`
	BiteDetector string `json:"bite_detector"`
	// RecordDir is the folder that holds one new directory per recording.
	RecordDir string `json:"record_dir"`
	// RecordKeys records keys in the selected window as well as the mouse.
	RecordKeys bool `json:"record_keys"`

	// Bite detection configuration (only actively used fields retained).
	ROISizePx int `json:"roi_size_px"` // square ROI side length in pixels
	// MaxCastDurationSeconds defines the maximum expected lifetime of a fishing cast (bobber present).
	// If monitoring exceeds this duration, the target is considered lost and the system returns to searching.
	MaxCastDurationSeconds int `json:"max_cast_duration_seconds"`
	// CooldownSeconds defines how long to wait after reeling before attempting the next cast.
	// 0, the default, casts again quickly, after a random 0.4 to 1.2 seconds.
	CooldownSeconds int `json:"cooldown_seconds"`

	// Theme is the window's look, one of Themes.
	Theme string `json:"theme"`

	// OverlayX and OverlayY place the overlay's status pill on the game window,
	// each as a share of the free space: 0 puts it against the left or top edge,
	// 1 against the right or bottom edge, 0.5 in the middle. The place holds when
	// the window changes size, and no place leaves the pill outside the window.
	OverlayX float64 `json:"overlay_x"`
	OverlayY float64 `json:"overlay_y"`
	// CatchesTotal counts every catch on this computer, for the thank-you card.
	// SupportAskedAt and SupportOpenedAt (Unix seconds, 0 for never) are when
	// the card last showed and when the person last opened the coffee page;
	// they space out the card (ui/desktop/support.go). Not on the Settings tab.
	CatchesTotal    int   `json:"catches_total"`
	SupportAskedAt  int64 `json:"support_asked_at"`
	SupportOpenedAt int64 `json:"support_opened_at"`
}

// Themes are the window's looks; the first is the default.
var Themes = []string{"graphite", "tavern", "arcane", "frost", "fel", "tide"}

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
		ReelMethod:             ReelRightClick,
		MaxFailedCasts:         5,
		MaxSessionMinutes:      60,
		PlayMinMinutes:         20,
		PlayMaxMinutes:         45,
		BreakMinMinutes:        5,
		BreakMaxMinutes:        15,
		Locator:                "marker",
		BiteDetector:           "splash_sound",
		RecordDir:              "datasets",
		RecordKeys:             false,
		ROISizePx:              80,
		MaxCastDurationSeconds: 30,
		CooldownSeconds:        0,
		Theme:                  Themes[0],
		OverlayX:               0.5,
		OverlayY:               0.01,
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
	if c.ReelMethod == "" {
		c.ReelMethod = d.ReelMethod
	}
	c.InteractKey = strings.TrimSpace(c.InteractKey)
	c.ESP32Port = strings.ToUpper(strings.TrimSpace(c.ESP32Port))
	if c.OverlayX < 0 || c.OverlayX > 1 {
		c.OverlayX = d.OverlayX
	}
	if c.OverlayY < 0 || c.OverlayY > 1 {
		c.OverlayY = d.OverlayY
	}
	c.TargetPlayerKey = strings.ToUpper(strings.TrimSpace(c.TargetPlayerKey))
	if c.MaxFailedCasts <= 0 {
		c.MaxFailedCasts = d.MaxFailedCasts
	}
	if c.MaxSessionMinutes <= 0 {
		c.MaxSessionMinutes = d.MaxSessionMinutes
	}
	if c.PlayMinMinutes == 0 {
		c.PlayMinMinutes = d.PlayMinMinutes
	}
	if c.PlayMaxMinutes == 0 {
		c.PlayMaxMinutes = d.PlayMaxMinutes
	}
	if c.BreakMinMinutes == 0 {
		c.BreakMinMinutes = d.BreakMinMinutes
	}
	if c.BreakMaxMinutes == 0 {
		c.BreakMaxMinutes = d.BreakMaxMinutes
	}
	c.LogoutKey = strings.TrimSpace(c.LogoutKey)
	if c.Locator == "" {
		c.Locator = d.Locator
	}
	if c.BiteDetector == "" || slices.Contains([]string{"motion", "splash", "histogram"}, c.BiteDetector) { // removed detectors
		c.BiteDetector = d.BiteDetector
	}
	if c.RecordDir == "" {
		c.RecordDir = d.RecordDir
	}
	c.ROISizePx = min(max(c.ROISizePx, 32), 256) // keep the ROI modest for performance
	c.MaxCastDurationSeconds = min(max(c.MaxCastDurationSeconds, 5), 180)
	c.CooldownSeconds = min(max(c.CooldownSeconds, 0), 60)
	c.Screen = max(c.Screen, 0)
	if !slices.Contains(Themes, c.Theme) {
		c.Theme = d.Theme
	}
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
