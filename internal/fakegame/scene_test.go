package fakegame

import (
	"image"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/fishing"
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
	x, y, found, err := capture.DetectTemplate(r.Render(PhaseBobber), tmpl, nil)
	if err != nil || !found {
		t.Fatalf("bobber not found: found=%v err=%v", found, err)
	}
	br := r.BobberRect()
	if x < br.Min.X-20 || x > br.Max.X+20 || y < br.Min.Y-20 || y > br.Max.Y+20 {
		t.Fatalf("match at (%d,%d) far from bobber %v", x, y, br)
	}
	if _, _, found, _ := capture.DetectTemplate(r.Render(PhaseEmpty), tmpl, nil); found {
		t.Fatalf("empty frame produced a false match")
	}
}

// Static bobber frames stay quiet; the splash frame triggers the real bite detector.
func TestBiteDetectorSeesSplashNotStatic(t *testing.T) {
	tmpl, _ := assets.FishingTargetImage()
	r := NewRenderer(DefaultScenario(), tmpl)
	br := r.BobberRect()
	cx, cy := (br.Min.X+br.Max.X)/2, (br.Min.Y+br.Max.Y)/2
	bd := fishing.NewBiteDetector(nil, nil)
	bd.Reset()
	start := time.Now()
	crop := func(p Phase) *image.RGBA { return cropROI(r.Render(p), cx, cy, 80) }
	for i := 0; i < 8; i++ {
		if bd.FeedFrame(crop(PhaseBobber), start.Add(time.Duration(i)*50*time.Millisecond)) {
			t.Fatalf("static frame %d triggered a bite", i)
		}
	}
	hit := false
	for i := 8; i < 11 && !hit; i++ {
		hit = bd.FeedFrame(crop(PhaseSplash), start.Add(time.Duration(i)*50*time.Millisecond))
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
