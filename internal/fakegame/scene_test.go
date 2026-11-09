package fakegame

import (
	"image"
	"math/rand"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/detect"
)

func TestPhaseAt(t *testing.T) {
	sc := DefaultScenario()
	cases := []struct {
		at   time.Duration
		want Phase
	}{
		{0, PhaseEmpty}, {2500 * time.Millisecond, PhaseBobber},
		{6200 * time.Millisecond, PhaseSplash}, {7000 * time.Millisecond, PhaseEmpty},
		{9600 * time.Millisecond, PhaseEmpty}, // wraps (total 9.6s) back to start
		{11700 * time.Millisecond, PhaseBobber},
	}
	for _, c := range cases {
		if got, _ := sc.PhaseAt(c.at); got != c.want {
			t.Errorf("PhaseAt(%v) = %v, want %v", c.at, got, c.want)
		}
	}
	sc.Loop = false
	if _, done := sc.PhaseAt(time.Hour); !done {
		t.Errorf("non-looping scenario should report done")
	}
}

// The real detector must find the bobber in a bobber frame and not in an empty one.
func TestDetectorFindsRenderedBobber(t *testing.T) {
	tmpl, err := assets.FishingTargetImage()
	if err != nil {
		t.Fatal(err)
	}
	r := NewRenderer(DefaultScenario(), tmpl)
	locator := detect.NewNCCLocator(tmpl, *config.DefaultConfig())
	m, found := locator.Locate(r.Render(PhaseBobber))
	if !found {
		t.Fatalf("bobber not found: %+v", m)
	}
	br := r.BobberRect()
	if m.X < br.Min.X-20 || m.X > br.Max.X+20 || m.Y < br.Min.Y-20 || m.Y > br.Max.Y+20 {
		t.Fatalf("match at (%d,%d) far from bobber %v", m.X, m.Y, br)
	}
	if _, found := locator.Locate(r.Render(PhaseEmpty)); found {
		t.Fatalf("empty frame produced a false match")
	}
}

// Static bobber frames stay quiet; the splash frame triggers the real bite detector.
func TestBiteDetectorSeesSplashNotStatic(t *testing.T) {
	tmpl, _ := assets.FishingTargetImage()
	r := NewRenderer(DefaultScenario(), tmpl)
	br := r.BobberRect()
	cx, cy := (br.Min.X+br.Max.X)/2, (br.Min.Y+br.Max.Y)/2
	bd := detect.NewMotionBiteDetector(nil)
	bd.Reset()
	start := time.Now()
	crop := func(p Phase) *image.RGBA { return cropROI(r.Render(p), cx, cy, 80) }
	// The detector ignores the landing, so the bobber floats for 2 s first.
	for i := 0; i < 40; i++ {
		if bd.Feed(crop(PhaseBobber), start.Add(time.Duration(i)*50*time.Millisecond)) {
			t.Fatalf("static frame %d triggered a bite", i)
		}
	}
	hit := false
	for i := 40; i < 43 && !hit; i++ {
		hit = bd.Feed(crop(PhaseSplash), start.Add(time.Duration(i)*50*time.Millisecond))
	}
	if !hit {
		t.Fatalf("splash did not trigger the bite detector")
	}
}

// cropROI copies a size x size square centered on (cx, cy) into a new image.
func cropROI(src *image.RGBA, cx, cy, size int) *image.RGBA {
	rect := image.Rect(cx-size/2, cy-size/2, cx+size/2, cy+size/2).Intersect(src.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		copy(out.Pix[y*out.Stride:(y+1)*out.Stride], src.Pix[src.PixOffset(rect.Min.X, rect.Min.Y+y):])
	}
	return out
}

// With a spread, each new bobber lands somewhere else, always with its
// brackets inside the scene; without one it stays home.
func TestLandSpreadsTheBobber(t *testing.T) {
	tmpl := image.NewRGBA(image.Rect(0, 0, 40, 40))
	sc := DefaultScenario()
	r := NewRenderer(sc, tmpl)
	rng := rand.New(rand.NewSource(1))
	r.Land(rng)
	if got := r.BobberRect().Min; got != image.Pt(sc.BobberX, sc.BobberY) {
		t.Fatalf("no spread moved the bobber to %v", got)
	}
	sc.Spread = 150
	r = NewRenderer(sc, tmpl)
	seen := map[image.Point]bool{}
	for range 50 {
		r.Land(rng)
		if !r.MarkerRect().In(image.Rect(0, 0, sc.Width, sc.Height)) {
			t.Fatalf("brackets %v leave the scene", r.MarkerRect())
		}
		seen[r.BobberRect().Min] = true
	}
	if len(seen) < 20 {
		t.Errorf("only %d places in 50 casts", len(seen))
	}
}
