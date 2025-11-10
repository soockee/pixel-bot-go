package images

import (
	"errors"
	"image"
	"image/draw"
)

// ExtractROI produces an ROI image centered at (cx, cy) with desired square side 'size'.
// It clamps the rectangle to frame bounds and guarantees at least 1x1.
// Returns the ROI image (always *image.RGBA) and the rectangle relative to frame.
func ExtractROI(frame *image.RGBA, cx, cy, size int) (*image.RGBA, image.Rectangle, error) {
	if frame == nil {
		return nil, image.Rectangle{}, errors.New("nil frame")
	}
	if size < 1 {
		size = 1
	}
	b := frame.Bounds()
	// initial top-left
	half := size / 2
	x0 := cx - half
	y0 := cy - half
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	// adjust width/height
	w := size
	h := size
	if x0+w > b.Dx() {
		w = b.Dx() - x0
	}
	if y0+h > b.Dy() {
		h = b.Dy() - y0
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	roi := image.Rect(x0, y0, x0+w, y0+h)
	sub := frame.SubImage(roi)
	if rgba, ok := sub.(*image.RGBA); ok {
		return rgba, roi, nil
	}
	out := image.NewRGBA(image.Rect(0, 0, roi.Dx(), roi.Dy()))
	draw.Draw(out, out.Bounds(), sub, roi.Min, draw.Src)
	return out, roi, nil
}
