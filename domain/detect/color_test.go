package detect

import (
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"testing"

	"github.com/soocke/pixel-bot-go/assets"
)

// lake returns a noisy 640x400 water scene with the bobber template drawn at
// the given top-left corner, or none when at is nil.
func lake(t *testing.T, at *image.Point) (*image.RGBA, image.Rectangle) {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, 640, 400))
	for y := range 400 {
		for x := range 640 {
			v := uint8(70 + rng.Intn(30))
			img.SetRGBA(x, y, color.RGBA{v, v + 10, v + 25, 255})
		}
	}
	if at == nil {
		return img, image.Rectangle{}
	}
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	r := tmpl.Bounds().Sub(tmpl.Bounds().Min).Add(*at)
	draw.Draw(img, r, tmpl, tmpl.Bounds().Min, draw.Over)
	return img, r
}

func TestColorLocatorFindsTheBobber(t *testing.T) {
	frame, bobber := lake(t, &image.Point{300, 150})
	m, ok := NewColorLocator().Locate(frame)
	if !ok {
		t.Fatal("no bobber found")
	}
	if !image.Pt(m.X, m.Y).In(bobber.Inset(-2)) {
		t.Errorf("click point (%d, %d) outside the bobber %v", m.X, m.Y, bobber)
	}
	if !bobber.In(m.Box) {
		t.Errorf("box %v does not cover the bobber %v", m.Box, bobber)
	}
}

func TestColorLocatorIgnoresPlainWater(t *testing.T) {
	frame, _ := lake(t, nil)
	if m, ok := NewColorLocator().Locate(frame); ok {
		t.Errorf("found a bobber in plain water at (%d, %d)", m.X, m.Y)
	}
}

func TestColorLocatorReportsFrameCoordinates(t *testing.T) {
	full, bobber := lake(t, &image.Point{300, 150})
	frame := full.SubImage(image.Rect(100, 50, 640, 400)).(*image.RGBA)
	m, ok := NewColorLocator().Locate(frame)
	if !ok {
		t.Fatal("no bobber found")
	}
	if want := bobber.Sub(image.Pt(100, 50)).Inset(-2); !image.Pt(m.X, m.Y).In(want) {
		t.Errorf("click point (%d, %d) outside %v", m.X, m.Y, want)
	}
}

func TestTanScorePrefersTheBody(t *testing.T) {
	if body, water := tanScore(200, 150, 110), tanScore(60, 75, 95); body <= water || body < colorTanMin {
		t.Errorf("body %v, water %v", body, water)
	}
}
