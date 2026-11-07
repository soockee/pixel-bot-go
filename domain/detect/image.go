package detect

import (
	"errors"
	"image"
)

// ExtractROI returns a square region-of-interest from the provided frame.
// The ROI is centered at (cx, cy) with side length "size" (minimum 1).
// The rectangle is clamped to the frame bounds and the result shares the
// frame's pixels. The returned rectangle is relative to the frame.
func ExtractROI(frame *image.RGBA, cx, cy, size int) (*image.RGBA, image.Rectangle, error) {
	if frame == nil {
		return nil, image.Rectangle{}, errors.New("nil frame")
	}
	size = max(size, 1)
	x0, y0 := max(cx-size/2, 0), max(cy-size/2, 0)
	r := image.Rect(x0, y0, x0+size, y0+size).Intersect(frame.Bounds())
	return frame.SubImage(r).(*image.RGBA), r, nil
}
