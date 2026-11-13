package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIsValid(t *testing.T) {
	got := DefaultConfig()
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if *got != *DefaultConfig() {
		t.Errorf("Validate changed the defaults: %+v", got)
	}
}

func TestValidateClamps(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Config)
		got  func(*Config) any
		want any
	}{
		{"roi floor", func(c *Config) { c.ROISizePx = 1 }, func(c *Config) any { return c.ROISizePx }, 32},
		{"roi ceiling", func(c *Config) { c.ROISizePx = 1000 }, func(c *Config) any { return c.ROISizePx }, 256},
		{"max cast floor", func(c *Config) { c.MaxCastDurationSeconds = 1 }, func(c *Config) any { return c.MaxCastDurationSeconds }, 5},
		{"max cast ceiling", func(c *Config) { c.MaxCastDurationSeconds = 999 }, func(c *Config) any { return c.MaxCastDurationSeconds }, 180},
		{"cooldown floor", func(c *Config) { c.CooldownSeconds = -1 }, func(c *Config) any { return c.CooldownSeconds }, 0},
		{"cooldown ceiling", func(c *Config) { c.CooldownSeconds = 999 }, func(c *Config) any { return c.CooldownSeconds }, 60},
		{"negative screen", func(c *Config) { c.Screen = -1 }, func(c *Config) any { return c.Screen }, 0},
		{"empty cast key", func(c *Config) { c.CastKey = "" }, func(c *Config) any { return c.CastKey }, "F9"},
		{"failed casts unset", func(c *Config) { c.MaxFailedCasts = 0 }, func(c *Config) any { return c.MaxFailedCasts }, 5},
		{"empty locator", func(c *Config) { c.Locator = "" }, func(c *Config) any { return c.Locator }, "marker"},
		{"empty bite detector", func(c *Config) { c.BiteDetector = "" }, func(c *Config) any { return c.BiteDetector }, "splash_sound"},
		{"removed bite detector", func(c *Config) { c.BiteDetector = "motion" }, func(c *Config) any { return c.BiteDetector }, "splash_sound"},
		{"empty record folder", func(c *Config) { c.RecordDir = "" }, func(c *Config) any { return c.RecordDir }, "datasets"},
		{"session limit unset", func(c *Config) { c.MaxSessionMinutes = 0 }, func(c *Config) any { return c.MaxSessionMinutes }, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			tt.set(c)
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := tt.got(c); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateFallsBackToTheDefaults(t *testing.T) {
	c := &Config{Threshold: 7, Theme: "dark"}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	d := DefaultConfig()
	if c.MinScale != d.MinScale || c.ScaleStep != d.ScaleStep || c.Threshold != d.Threshold || c.Stride != d.Stride || c.Theme != d.Theme {
		t.Errorf("got %+v, want the defaults", c)
	}
	if c.MaxScale < c.MinScale {
		t.Errorf("MaxScale = %v is below MinScale", c.MaxScale)
	}
}

func TestValidateKeepsASingleScale(t *testing.T) {
	c := DefaultConfig()
	c.MinScale, c.MaxScale, c.ScaleStep = 1, 1, 0.05
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.ScaleStep <= 0 {
		t.Errorf("ScaleStep = %v, a zero step disables the search", c.ScaleStep)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if *got != *DefaultConfig() {
		t.Errorf("got %+v", got)
	}
}

func TestLoadInvalidJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err == nil {
		t.Fatal("want an error")
	}
	if got == nil {
		t.Fatal("want a usable config alongside the error")
	}
}

func TestLoadKeepsDefaultsAndClampsValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.json")
	if err := os.WriteFile(path, []byte(`{"roi_size_px": 1000, "background": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ROISizePx != 256 {
		t.Errorf("ROISizePx = %d, want the clamped 256", got.ROISizePx)
	}
	if got.Threshold != DefaultConfig().Threshold {
		t.Errorf("Threshold = %v, want the default", got.Threshold)
	}
	if err := got.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"background"`)) {
		t.Error("saved the retired background setting")
	}
}

func TestLoadKeepsOldReelKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(path, []byte(`{"reel_key": "R"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CastKey != "R" {
		t.Errorf("CastKey = %q, want R", got.CastKey)
	}
	if got.ReelMethod != ReelRightClick || got.InteractKey != DefaultConfig().InteractKey {
		t.Errorf("old config reel settings = %q, %q", got.ReelMethod, got.InteractKey)
	}
}

func TestSaveRoundTripSkipsRuntimeFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := DefaultConfig()
	want.CastKey = "R"
	want.ReelMethod, want.InteractKey = ReelInteractKey, "F"
	saved := *want
	saved.DryRun, saved.AutoStart, saved.AutoWindow = true, true, "Fake Game"
	if err := saved.Save(path); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
