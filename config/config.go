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
}

// DefaultConfig returns a Config populated with standard defaults.
func DefaultConfig() *Config {
	return &Config{
		Debug:          false,
		MinScale:       0.60,
		MaxScale:       1.40,
		ScaleStep:      0.05,
		Threshold:      0.80,
		Stride:         4,
		Refine:         true,
		UseRGB:         true,
		StopOnScore:    0.95,
		ReturnBestEven: true,
		SelectionX:     0,
		SelectionY:     0,
		SelectionW:     0,
		SelectionH:     0,
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
