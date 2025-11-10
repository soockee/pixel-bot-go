package capture

import (
	"errors"
	"image"

	"github.com/soocke/pixel-bot-go/config"
)

// DetectTemplate performs a multi-scale NCC template match using dynamic config.
// Returns x,y,ok where (x,y) is the top-left of the best match whose NCC score
// meets the configured threshold. Transparent template pixels are masked out.
func DetectTemplate(frame *image.RGBA, tmpl image.Image, cfg *config.Config) (int, int, bool, error) {
	if frame == nil || tmpl == nil {
		return 0, 0, false, errors.New("detect template error")
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	} else if err := cfg.Validate(); err != nil {
		return 0, 0, false, err
	}
	ms := MultiScaleMatch(frame, tmpl, MultiScaleOptions{
		Scales:    nil,
		MinScale:  cfg.MinScale,
		MaxScale:  cfg.MaxScale,
		ScaleStep: cfg.ScaleStep,
		NCC: NCCOptions{
			Threshold:      cfg.Threshold,
			Stride:         cfg.Stride,
			Refine:         cfg.Refine,
			ReturnBestEven: cfg.ReturnBestEven,
		},
		StopOnScore: cfg.StopOnScore,
	})
	return ms.X, ms.Y, ms.Found, nil
}
