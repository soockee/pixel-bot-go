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

func TestTheAppsDetectorsAreRegistered(t *testing.T) {
	if !slices.Contains(LocatorNames(), "marker") || !slices.Contains(BiteDetectorNames(), SplashSoundName) {
		t.Errorf("locators %v, bite detectors %v; want marker and %s", LocatorNames(), BiteDetectorNames(), SplashSoundName)
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
