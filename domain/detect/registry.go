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
		"cast_track": func(config.Config) (Locator, error) { return &castTrackLocator{}, nil },
		"color":      func(config.Config) (Locator, error) { return NewColorLocator(), nil },
		"marker":     func(config.Config) (Locator, error) { return NewMarkerLocator(), nil },
		"ncc": func(cfg config.Config) (Locator, error) {
			tmpl, err := assets.FishingTargetImage()
			if err != nil {
				return nil, fmt.Errorf("load bobber template: %w", err)
			}
			return NewNCCLocator(tmpl, cfg), nil
		},
	}
	biteDetectors = map[string]func(*slog.Logger) (BiteDetector, error){
		"splash_sound":          splashVariant(false, false),
		"splash_history":        splashVariant(true, false),
		"splash_template_sound": splashVariant(false, true),
		"splash_history_sound":  splashVariant(true, true),
	}
)

func splashVariant(history, template bool) func(*slog.Logger) (BiteDetector, error) {
	return func(logger *slog.Logger) (BiteDetector, error) {
		d := NewSplashSoundDetector(logger)
		if history {
			d.history = &sprayHistory{}
		}
		if template {
			sounds, err := assets.BiteSoundPCM()
			if err != nil {
				return nil, fmt.Errorf("load bite templates: %w", err)
			}
			d.templates = newBiteTemplates(sounds)
		}
		return d, nil
	}
}

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
	return build(logger)
}
