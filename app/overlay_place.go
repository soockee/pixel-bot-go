package app

import (
	"image"
	"math"
)

// The pill has one size, in device-independent pixels, so it does not jump when
// the person is done placing it (frontend/src/overlay.css uses the same).
// While it is moved its window is only the pill, so the rest of the game stays
// clickable.
const (
	pillW = 380
	pillH = 44
)

// pillRect is where a w by h pill sits on area for the place (x, y). Each is a
// fraction of the free space: 0 puts the pill against the left or top edge, 1
// against the right or bottom edge, so no place can leave the pill outside the
// area.
func pillRect(area image.Rectangle, x, y float64, w, h int) image.Rectangle {
	left := area.Min.X + int(math.Round(x*float64(max(0, area.Dx()-w))))
	top := area.Min.Y + int(math.Round(y*float64(max(0, area.Dy()-h))))
	left = max(area.Min.X, min(left, area.Max.X-w))
	top = max(area.Min.Y, min(top, area.Max.Y-h))
	return image.Rect(left, top, left+w, top+h)
}

// pillPlace is the place of a pill at pill on area, the inverse of pillRect. A
// pill dragged partly outside the area is pulled back inside first. It reports
// false when there is no area or pill.
func pillPlace(pill, area image.Rectangle) (x, y float64, ok bool) {
	if area.Empty() || pill.Empty() {
		return 0, 0, false
	}
	w, h := pill.Dx(), pill.Dy()
	left := max(area.Min.X, min(pill.Min.X, area.Max.X-w))
	top := max(area.Min.Y, min(pill.Min.Y, area.Max.Y-h))
	return fraction(left-area.Min.X, area.Dx()-w), fraction(top-area.Min.Y, area.Dy()-h), true
}

// fraction is offset as a share of the free space; no space leaves it at 0.
func fraction(offset, free int) float64 {
	if free <= 0 {
		return 0
	}
	return float64(offset) / float64(free)
}
