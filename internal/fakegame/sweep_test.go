package fakegame

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
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
	locator := detect.NewNCCLocator(tmpl, *config.DefaultConfig())
	misses := 0
	for dy := 0; dy < 6; dy++ {
		for dx := 0; dx < 6; dx++ {
			sc := DefaultScenario()
			sc.BobberX, sc.BobberY = 200+dx, 120+dy
			frame := desktopFrame(NewRenderer(sc, tmpl).Render(PhaseBobber), 13, 31)
			m, found := locator.Locate(frame)
			b := tmpl.Bounds()
			wantX, wantY := 13+sc.BobberX+b.Dx()/2, 31+sc.BobberY+b.Dy()/2
			if !found || m.X < wantX-3 || m.X > wantX+3 || m.Y < wantY-3 || m.Y > wantY+3 {
				misses++
				t.Errorf("bobber at (%d,%d): found=%v at (%d,%d)", wantX, wantY, found, m.X, m.Y)
			}
		}
	}
	t.Logf("misses: %d/36", misses)
}
