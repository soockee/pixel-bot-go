package config

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
