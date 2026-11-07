package fakegame

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// desktopFrame embeds the scene in a 560x420 frame with a smooth orange
// gradient around it, like a window sitting on a wallpaper.
func desktopFrame(scene *image.RGBA, ox, oy int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, 560, 420))
	for y := 0; y < 420; y++ {
		for x := 0; x < 560; x++ {
			out.SetRGBA(x, y, color.RGBA{uint8(200 + x/20), uint8(110 + y/12), uint8(30 + (x+y)/30), 255})
		}
	}
	draw.Draw(out, image.Rect(ox, oy, ox+scene.Bounds().Dx(), oy+scene.Bounds().Dy()), scene, image.Point{}, draw.Src)
	return out
}

// With the default config the matcher must find the bobber at every pixel
// alignment. A coarse stride once skipped the true position and missed it.
func TestDetectorFindsBobberAtEveryAlignment(t *testing.T) {
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	misses := 0
	for dy := 0; dy < 6; dy++ {
		for dx := 0; dx < 6; dx++ {
			sc := DefaultScenario()
			sc.BobberX, sc.BobberY = 200+dx, 120+dy
			frame := desktopFrame(NewRenderer(sc, tmpl).Render(PhaseBobber), 13, 31)
			x, y, found, err := capture.DetectTemplate(frame, tmpl, nil)
			wantX, wantY := 13+sc.BobberX, 31+sc.BobberY
			if err != nil || !found || x < wantX-3 || x > wantX+3 || y < wantY-3 || y > wantY+3 {
				misses++
				t.Errorf("bobber at (%d,%d): found=%v at (%d,%d)", wantX, wantY, found, x, y)
			}
		}
	}
	t.Logf("misses: %d/36", misses)
}
