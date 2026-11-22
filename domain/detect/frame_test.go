package detect

import (
	"image"
	"image/color"
	"math/rand"
)

const frameW, frameH = 200, 150

// noiseFrame is a repeatable grey noise frame for the locator tests.
func noiseFrame() *image.RGBA {
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, frameW, frameH))
	for y := range frameH {
		for x := range frameW {
			v := uint8(rng.Intn(256))
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}
