package app

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestScheduleImageRequiresEveryPixel(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 60, 50))
	crop := image.NewRGBA(image.Rect(0, 0, 8, 9))
	draw.Draw(crop, crop.Bounds(), image.NewUniform(color.RGBA{50, 60, 70, 255}), image.Point{}, draw.Src)
	crop.SetRGBA(7, 8, color.RGBA{200, 100, 50, 255})
	at := image.Pt(41, 31)
	draw.Draw(frame, crop.Bounds().Add(at), crop, image.Point{}, draw.Src)
	sub := frame.SubImage(image.Rect(20, 20, 52, 43)).(*image.RGBA)
	if !containsScreenPatch(sub, crop) {
		t.Fatal("missed matching control in a strided frame")
	}
	frame.SetRGBA(at.X+7, at.Y+8, color.RGBA{200, 100, 51, 255})
	if containsScreenPatch(sub, crop) {
		t.Fatal("accepted a partial match")
	}
}

func TestScheduleDurationStaysInsideRange(t *testing.T) {
	if got := randomDuration(3, 3); got != 3*time.Minute {
		t.Fatalf("fixed interval = %s", got)
	}
	for range 100 {
		if got := randomDuration(2, 4); got < 2*time.Minute || got > 4*time.Minute {
			t.Fatalf("sample outside interval: %s", got)
		}
	}
}
