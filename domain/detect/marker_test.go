package detect

import (
	"image"
	"image/color"
	"testing"
)

var testBox = image.Rect(40, 30, 143, 122)

func TestMarkerLocatorFindsTheBox(t *testing.T) {
	frame := noiseFrame()
	DrawMarker(frame, testBox)

	got, found := NewMarkerLocator().Locate(frame)

	if !found || got.Box != testBox || got.Score != 1 {
		t.Fatalf("got %+v found=%v, want box %v", got, found, testBox)
	}
	if got.X != 91 || got.Y != 76 {
		t.Errorf("centre = (%d,%d), want (91,76)", got.X, got.Y)
	}
}

// paint fills a rectangle, like an effect covering part of the marker.
func paint(frame *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			frame.SetRGBA(x, y, c)
		}
	}
}

// Older recordings show NaowhForever Glimmer's brackets, which take on the
// scene's colour; values measured in real frames.
func TestMarkerLocatorFindsGlimmerBrackets(t *testing.T) {
	for name, c := range map[string]color.RGBA{
		"day":   {255, 178, 163, 255},
		"night": {255, 144, 141, 255},
	} {
		t.Run(name, func(t *testing.T) {
			frame := noiseFrame()
			DrawMarker(frame, testBox)
			for i := 0; i < len(frame.Pix); i += 4 {
				if frame.Pix[i] == markerColor.R && frame.Pix[i+1] == markerColor.G {
					frame.Pix[i], frame.Pix[i+1], frame.Pix[i+2] = c.R, c.G, c.B
				}
			}
			if got, found := NewMarkerLocator().Locate(frame); !found || got.Box != testBox {
				t.Errorf("got %+v found=%v, want box %v", got, found, testBox)
			}
		})
	}
}

func TestMarkerLocatorSurvivesOneCoveredCorner(t *testing.T) {
	grey := color.RGBA{60, 60, 60, 255}
	covers := map[string]image.Rectangle{
		"top left":     image.Rect(35, 25, 65, 55),
		"top right":    image.Rect(120, 25, 150, 55),
		"bottom left":  image.Rect(35, 100, 65, 130),
		"bottom right": image.Rect(120, 100, 150, 130),
	}
	for name, cover := range covers {
		t.Run(name, func(t *testing.T) {
			frame := noiseFrame()
			DrawMarker(frame, testBox)
			paint(frame, cover, grey)

			got, found := NewMarkerLocator().Locate(frame)

			if !found || got.Box != testBox || got.Score != 0.75 {
				t.Errorf("got %+v found=%v", got, found)
			}
		})
	}
}

func TestMarkerLocatorRejects(t *testing.T) {
	grey := color.RGBA{60, 60, 60, 255}
	tests := []struct {
		name string
		draw func(*image.RGBA)
	}{
		{"nothing", func(*image.RGBA) {}},
		{"two corners", func(f *image.RGBA) {
			DrawMarker(f, testBox)
			paint(f, image.Rect(35, 25, 150, 55), grey)
		}},
		{"another colour", func(f *image.RGBA) {
			for _, r := range []image.Rectangle{testBox} {
				DrawMarker(f, r)
			}
			for i := 0; i < len(f.Pix); i += 4 {
				if f.Pix[i] == markerColor.R && f.Pix[i+1] == markerColor.G {
					f.Pix[i], f.Pix[i+1], f.Pix[i+2] = 255, 215, 0
				}
			}
		}},
		{"too small", func(f *image.RGBA) { DrawMarker(f, image.Rect(10, 10, 26, 26)) }},
		{"sparkle in the same colour", func(f *image.RGBA) {
			paint(f, image.Rect(100, 60, 103, 100), markerColor)
			paint(f, image.Rect(80, 78, 124, 81), markerColor)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := noiseFrame()
			tt.draw(frame)
			if got, found := NewMarkerLocator().Locate(frame); found {
				t.Errorf("got a match: %+v", got)
			}
		})
	}
}

func TestMarkerLocatorHandlesAMarkerCutByTheFrame(t *testing.T) {
	box := image.Rect(frameW-90, 20, frameW+30, 120) // right corners are outside
	frame := noiseFrame()
	DrawMarker(frame, box)
	if _, found := NewMarkerLocator().Locate(frame); found {
		t.Error("two visible corners must not count as a marker")
	}
}

func TestMarkerLocatorReusesItsBufferAcrossFrameSizes(t *testing.T) {
	locator := NewMarkerLocator()
	large := noiseFrame()
	DrawMarker(large, testBox)
	if _, found := locator.Locate(large); !found {
		t.Fatal("large frame: no match")
	}

	small := image.NewRGBA(image.Rect(0, 0, 60, 60))
	if got, found := locator.Locate(small); found {
		t.Errorf("small frame saw old data: %+v", got)
	}
}
