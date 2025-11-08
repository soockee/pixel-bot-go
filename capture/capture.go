package capture

import (
	"image"

	"github.com/vova616/screenshot"
)

// Grab returns a screen capture of the current active monitor
func Grab() (*image.RGBA, error) {
	img, err := screenshot.CaptureScreen()
	if err != nil {
		return nil, err
	}
	return img, nil
}

// DetectTemplate performs a normalized cross-correlation (NCC) template match.
// Returns x,y,ok where (x,y) is the top-left of the best match whose NCC score
// exceeds the internal threshold. Transparent pixels (alpha==0) in the template
// are ignored (masked out).
func DetectTemplate(frame *image.RGBA, tmpl image.Image) (int, int, bool) {
	// Multi-scale match to handle size variability. Defaults tuned for moderate UI scaling.
	ms := MultiScaleMatch(frame, tmpl, MultiScaleOptions{
		// Leave Scales empty to auto-generate from MinScale..MaxScale
		MinScale:  0.60,
		MaxScale:  1.40,
		ScaleStep: 0.05,
		NCC: NCCOptions{
			Threshold:      0.80,
			Stride:         4,
			Refine:         true,
			ReturnBestEven: true,
			UseRGB:         true,
		},
		StopOnScore: 0.95,
	})
	return ms.X, ms.Y, ms.Found
}
