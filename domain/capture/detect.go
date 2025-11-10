package capture

import (
	"errors"
	"image"

	"github.com/soocke/pixel-bot-go/config"
)

// DetectTemplateDetailed performs a multi-scale normalized cross-correlation
// (NCC) template match and returns the full result, including timing and scale
// counts when DebugTiming is enabled. Transparent template pixels are masked
// during matching.
func DetectTemplateDetailed(frame *image.RGBA, tmpl image.Image, cfg *config.Config) (MultiScaleResult, error) {
	if frame == nil || tmpl == nil {
		return MultiScaleResult{}, errors.New("detect template error")
	}
	var local config.Config
	if cfg == nil {
		local = *config.DefaultConfig()
	} else {
		local = *cfg
	}
	if err := local.Validate(); err != nil {
		return MultiScaleResult{}, err
	}
	res := MultiScaleMatch(frame, tmpl, MultiScaleOptions{
		Scales:    nil,
		MinScale:  local.MinScale,
		MaxScale:  local.MaxScale,
		ScaleStep: local.ScaleStep,
		NCC: NCCOptions{
			Threshold:      local.Threshold,
			Stride:         local.Stride,
			Refine:         local.Refine,
			ReturnBestEven: local.ReturnBestEven,
			DebugTiming:    true,
		},
		StopOnScore: local.StopOnScore,
	})
	return res, nil
}

// DetectTemplate is a compatibility helper that returns coordinates and a
// boolean found flag.
func DetectTemplate(frame *image.RGBA, tmpl image.Image, cfg *config.Config) (int, int, bool, error) {
	res, err := DetectTemplateDetailed(frame, tmpl, cfg)
	if err != nil {
		return 0, 0, false, err
	}
	return res.X, res.Y, res.Found, nil
}
