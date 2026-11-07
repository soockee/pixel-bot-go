package detect

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"testing"

	"github.com/soocke/pixel-bot-go/config"
)

const (
	tmplW, tmplH   = 24, 16
	frameW, frameH = 200, 150
)

// pattern is a smooth surface, so a coarse scan still scores well near the
// true position. A scale stretches it, like an object seen closer up.
func pattern(x, y int, scale float64) uint8 {
	v := 128 + 100*math.Sin(float64(x)/(5*scale))*math.Cos(float64(y)/(4*scale))
	return uint8(v)
}

func patternImage(scale float64) *image.RGBA {
	w, h := int(tmplW*scale), int(tmplH*scale)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := pattern(x, y, scale)
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

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

func paste(frame, src *image.RGBA, atX, atY int) {
	b := src.Bounds()
	for y := range b.Dy() {
		for x := range b.Dx() {
			frame.SetRGBA(atX+x, atY+y, src.RGBAAt(x, y))
		}
	}
}

func matchOne(frame *image.RGBA, tmpl image.Image, opts NCCOptions) NCCResult {
	var pre grayPrecomp
	pre.load(frame)
	return matchTemplate(&pre, newTemplatePrecomp(tmpl), opts)
}

func TestMatchTemplateFindsExactPosition(t *testing.T) {
	tests := []struct {
		name   string
		x, y   int
		stride int
		refine bool
	}{
		{"full scan", 73, 41, 1, false},
		{"top left corner", 0, 0, 1, false},
		{"bottom right corner", frameW - tmplW, frameH - tmplH, 1, false},
		{"coarse scan with refine", 73, 41, 3, true},
		{"refine in the bottom right corner", frameW - tmplW, frameH - tmplH, 3, true},
		{"refine in the top left corner", 0, 0, 3, true},
		{"oversized stride is capped", 75, 45, 100, true},
	}
	tmpl := patternImage(1)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := noiseFrame()
			paste(frame, tmpl, tt.x, tt.y)
			got := matchOne(frame, tmpl, NCCOptions{Threshold: 0.9, Stride: tt.stride, Refine: tt.refine})
			if !got.Found || got.X != tt.x || got.Y != tt.y {
				t.Errorf("got %+v, want a match at (%d,%d)", got, tt.x, tt.y)
			}
			if got.Score < 0.999 {
				t.Errorf("score = %v, want about 1", got.Score)
			}
		})
	}
}

func TestMatchTemplateRejects(t *testing.T) {
	tmpl := patternImage(1)
	flat := image.NewRGBA(image.Rect(0, 0, frameW, frameH))
	tiny := image.NewRGBA(image.Rect(0, 0, tmplW-1, tmplH))
	tests := []struct {
		name  string
		frame *image.RGBA
	}{
		{"unrelated frame", noiseFrame()},
		{"flat frame", flat},
		{"frame smaller than template", tiny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchOne(tt.frame, tmpl, NCCOptions{Threshold: 0.9, Stride: 1})
			if got.Found {
				t.Errorf("got a match: %+v", got)
			}
		})
	}
}

func TestMultiScaleMatchPicksTheRightScale(t *testing.T) {
	frame := noiseFrame()
	paste(frame, patternImage(1.25), 60, 50)

	var pre grayPrecomp
	pre.load(frame)
	templates := prepareTemplates(patternImage(1), []float64{1, 1.25, 1.5})
	got := matchScales(&pre, templates, NCCOptions{Threshold: 0.9, Stride: 1})

	if !got.Found || got.Scale != 1.25 || got.X != 60 || got.Y != 50 {
		t.Errorf("got %+v, want scale 1.25 at (60,50)", got)
	}
}

func TestNCCLocatorReturnsTheCentreOfTheMatch(t *testing.T) {
	frame := noiseFrame()
	paste(frame, patternImage(1), 60, 50)
	cfg := *config.DefaultConfig()
	cfg.MinScale, cfg.MaxScale, cfg.ScaleStep = 1, 1, 1

	got, found := NewNCCLocator(patternImage(1), cfg).Locate(frame)

	if !found || got.X != 60+tmplW/2 || got.Y != 50+tmplH/2 {
		t.Errorf("got %+v found=%v, want the centre (%d,%d)", got, found, 60+tmplW/2, 50+tmplH/2)
	}
}

// A flat template has nothing to correlate with.
func TestFlatTemplateNeverMatches(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, tmplW, tmplH))
	frame := noiseFrame()
	paste(frame, flat, 40, 40)
	if got := matchOne(frame, flat, NCCOptions{Threshold: 0.1, Stride: 1}); got.Found {
		t.Errorf("got a match: %+v", got)
	}
}

// Buffers are reused, so a smaller frame after a larger one must not see old data.
func TestLocatorHandlesChangingFrameSizes(t *testing.T) {
	locator := NewNCCLocator(patternImage(1), exactScaleConfig())
	frame := noiseFrame()
	paste(frame, patternImage(1), 60, 50)
	want, _ := locator.Locate(frame)

	small := image.NewRGBA(image.Rect(0, 0, 100, 80))
	paste(small, patternImage(1), 10, 20)
	got, found := locator.Locate(small)
	if !found || got.X != 10+tmplW/2 || got.Y != 20+tmplH/2 {
		t.Errorf("small frame: %+v found=%v", got, found)
	}
	if again, _ := locator.Locate(frame); again != want {
		t.Errorf("large frame after small: %+v, want %+v", again, want)
	}
}

func BenchmarkLocate(b *testing.B) {
	frame := noiseFrame()
	paste(frame, patternImage(1), 60, 50)
	locator := NewNCCLocator(patternImage(1), *config.DefaultConfig())
	b.ReportAllocs()
	for b.Loop() {
		locator.Locate(frame)
	}
}

func exactScaleConfig() config.Config {
	cfg := *config.DefaultConfig()
	cfg.MinScale, cfg.MaxScale = 1, 1
	return cfg
}

// Two templates of one size must not share prepared data (DET-8).
func TestLocatorsWithSameSizedTemplatesStayApart(t *testing.T) {
	first, second := patternImage(1), patternImage(1)
	for i := range second.Pix {
		if i%4 != 3 {
			second.Pix[i] = 255 - second.Pix[i]
		}
	}
	frame := noiseFrame()
	paste(frame, first, 20, 20)
	paste(frame, second, 120, 90)

	a, _ := NewNCCLocator(first, exactScaleConfig()).Locate(frame)
	b, _ := NewNCCLocator(second, exactScaleConfig()).Locate(frame)

	if a.X != 20+tmplW/2 || b.X != 120+tmplW/2 {
		t.Errorf("first at x=%d, second at x=%d, want %d and %d", a.X, b.X, 20+tmplW/2, 120+tmplW/2)
	}
}

// Evaluation needs one frame to give one answer (DET-9).
func TestLocateIsRepeatable(t *testing.T) {
	frame := noiseFrame()
	paste(frame, patternImage(1), 60, 50)
	locator := NewNCCLocator(patternImage(1), *config.DefaultConfig())

	want, wantFound := locator.Locate(frame)
	for range 20 {
		if got, found := locator.Locate(frame); got != want || found != wantFound {
			t.Fatalf("got %+v found=%v, want %+v found=%v", got, found, want, wantFound)
		}
	}
}

func TestIntegralSum(t *testing.T) {
	// Summed-area table of [1 2; 3 4].
	table := []float64{1, 3, 4, 10}
	tests := []struct {
		name           string
		x0, y0, x1, y1 int
		want           float64
	}{
		{"whole image", 0, 0, 1, 1, 10},
		{"single cell", 1, 1, 1, 1, 4},
		{"right column", 1, 0, 1, 1, 6},
		{"bottom row", 0, 1, 1, 1, 7},
		{"inverted rectangle", 1, 1, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := integralSum(table, 2, tt.x0, tt.y0, tt.x1, tt.y1); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
