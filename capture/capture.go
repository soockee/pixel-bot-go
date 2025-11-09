package capture

import (
	"image"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/vova616/screenshot"
)

// Grab returns a screen capture of the current active monitor.
func Grab() (*image.RGBA, error) {
	img, err := screenshot.CaptureScreen()

	screenshot.ScreenRect()
	if err != nil {
		return nil, err
	}
	return img, nil
}

func GrabSelection(selecationArea image.Rectangle) (*image.RGBA, error) {
	img, err := screenshot.CaptureRect(selecationArea)

	if err != nil {
		return nil, err
	}
	return img, nil
}

// DetectTemplate performs a multi-scale NCC template match using dynamic config.
// Returns x,y,ok where (x,y) is the top-left of the best match whose NCC score
// meets the configured threshold. Transparent template pixels are masked out.
func DetectTemplate(frame *image.RGBA, tmpl image.Image, cfg *config.Config) (int, int, bool) {
	if frame == nil || tmpl == nil {
		return 0, 0, false
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	ms := MultiScaleMatch(frame, tmpl, MultiScaleOptions{
		Scales:    nil, // force adaptive generation path
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
	return ms.X, ms.Y, ms.Found
}
