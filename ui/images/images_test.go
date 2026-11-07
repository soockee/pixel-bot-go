package images

import (
	"image"
	"image/color"
	"testing"
)

func TestScaleToFitKeepsTheAspectRatio(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	if got := ScaleToFit(src, 100, 100).Bounds().Size(); got != image.Pt(100, 50) {
		t.Errorf("size = %v, want 100x50", got)
	}
}

func TestScaleToFitNeverEnlarges(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	if got := ScaleToFit(src, 100, 100).Bounds().Size(); got != image.Pt(40, 20) {
		t.Errorf("size = %v, want 40x20", got)
	}
}

func TestScaleToFitSamplesTheRightPixels(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	for y := range 4 {
		for x := range 4 {
			c := red
			if x >= 2 {
				c = blue
			}
			src.SetRGBA(x, y, c)
		}
	}
	got := ScaleToFit(src.SubImage(image.Rect(0, 0, 4, 4)).(*image.RGBA), 2, 2)
	if got.RGBAAt(0, 0) != red || got.RGBAAt(1, 1) != blue {
		t.Errorf("got %v and %v", got.RGBAAt(0, 0), got.RGBAAt(1, 1))
	}
}
