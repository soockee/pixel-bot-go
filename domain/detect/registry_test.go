package detect

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// Every registered technique must build and run on a plain frame.
func TestEveryRegisteredDetectorRuns(t *testing.T) {
	frame := noiseFrame()
	for _, name := range LocatorNames() {
		t.Run(name, func(t *testing.T) {
			l, err := NewLocator(name, *config.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			l.Locate(frame)
		})
	}
	for _, name := range BiteDetectorNames() {
		t.Run(name, func(t *testing.T) {
			b, err := NewBiteDetector(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			b.Reset()
			b.Feed(frame.SubImage(image.Rect(0, 0, 40, 40)).(*image.RGBA), time.Now())
		})
	}
}

func TestDefaultsAreRegistered(t *testing.T) {
	cfg := config.DefaultConfig()
	if !slices.Contains(LocatorNames(), cfg.Locator) {
		t.Errorf("default locator %q is not registered", cfg.Locator)
	}
	if !slices.Contains(BiteDetectorNames(), cfg.BiteDetector) {
		t.Errorf("default bite detector %q is not registered", cfg.BiteDetector)
	}
}

func TestUnknownNamesAreRefused(t *testing.T) {
	if _, err := NewLocator("nope", *config.DefaultConfig()); err == nil {
		t.Error("locator: want an error")
	}
	if _, err := NewBiteDetector("nope", nil); err == nil {
		t.Error("bite detector: want an error")
	}
}
