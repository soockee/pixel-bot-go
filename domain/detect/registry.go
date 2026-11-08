package detect

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
)

// To add a technique, implement Locator or BiteDetector and add one line here.
// The app config, the config panel and detecteval all pick it up by name.
var (
	locators = map[string]func(config.Config) (Locator, error){
		"marker": func(config.Config) (Locator, error) { return NewMarkerLocator(), nil },
		"ncc": func(cfg config.Config) (Locator, error) {
			tmpl, err := assets.FishingTargetImage()
			if err != nil {
				return nil, fmt.Errorf("load bobber template: %w", err)
			}
			return NewNCCLocator(tmpl, cfg), nil
		},
	}
	biteDetectors = map[string]func(*slog.Logger) BiteDetector{
		"motion":       func(logger *slog.Logger) BiteDetector { return NewMotionBiteDetector(logger) },
		"splash":       func(logger *slog.Logger) BiteDetector { return NewSplashBiteDetector(logger) },
		"splash_sound": func(logger *slog.Logger) BiteDetector { return NewSplashSoundDetector(logger) },
	}
)

// LocatorNames lists the registered locators.
func LocatorNames() []string { return slices.Sorted(maps.Keys(locators)) }

// BiteDetectorNames lists the registered bite detectors.
func BiteDetectorNames() []string { return slices.Sorted(maps.Keys(biteDetectors)) }

// NewLocator builds a registered locator.
func NewLocator(name string, cfg config.Config) (Locator, error) {
	build, ok := locators[name]
	if !ok {
		return nil, fmt.Errorf("unknown locator %q", name)
	}
	return build(cfg)
}

// NewBiteDetector builds a registered bite detector.
func NewBiteDetector(name string, logger *slog.Logger) (BiteDetector, error) {
	build, ok := biteDetectors[name]
	if !ok {
		return nil, fmt.Errorf("unknown bite detector %q", name)
	}
	return build(logger), nil
}
