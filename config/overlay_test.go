package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayPlaceDefaultsToTheTopCentre(t *testing.T) {
	c := DefaultConfig()
	if c.OverlayX != 0.5 || c.OverlayY != 0.01 {
		t.Fatalf("default place = %v, %v", c.OverlayX, c.OverlayY)
	}
}

func TestValidateKeepsAnOverlayPlaceInsideTheWindow(t *testing.T) {
	for _, tc := range []struct {
		name         string
		x, y         float64
		wantX, wantY float64
	}{
		{"inside", 0.2, 0.7, 0.2, 0.7},
		{"a corner is a place", 0, 1, 0, 1},
		{"left of the window", -0.1, 0.5, 0.5, 0.5},
		{"below the window", 0.3, 1.5, 0.3, 0.01},
	} {
		c := DefaultConfig()
		c.OverlayX, c.OverlayY = tc.x, tc.y
		_ = c.Validate()
		if c.OverlayX != tc.wantX || c.OverlayY != tc.wantY {
			t.Errorf("%s: place = %v, %v; want %v, %v", tc.name, c.OverlayX, c.OverlayY, tc.wantX, tc.wantY)
		}
	}
}

func TestLoadGivesAnOldFileTheDefaultOverlayPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"cast_key": "R"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.OverlayX != 0.5 || got.OverlayY != 0.01 {
		t.Fatalf("a file saved before the overlay could move = %v, %v; want the top centre", got.OverlayX, got.OverlayY)
	}
}

func TestOverlayPlaceSurvivesSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := DefaultConfig()
	c.OverlayX, c.OverlayY = 0.12, 0.83
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.OverlayX != 0.12 || got.OverlayY != 0.83 {
		t.Fatalf("place after a round trip = %v, %v", got.OverlayX, got.OverlayY)
	}
}
