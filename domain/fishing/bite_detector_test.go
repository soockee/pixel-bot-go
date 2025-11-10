package fishing

import (
	"image"
	"testing"
	"time"
)

// synthFrame creates a uniform RGBA image and applies an optional mutate func.
func synthFrame(w, h int, base byte, mutate func(px []byte, w, h int)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = base, base, base, 255
		}
	}
	if mutate != nil {
		mutate(img.Pix, w, h)
	}
	return img
}

// applyRegion sets RGB values to 'lum' inside the given rectangle (clamped).
func applyRegion(px []byte, w, h int, x0, y0, x1, y1 int, lum byte) {
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > w {
		x1 = w
	}
	if y1 > h {
		y1 = h
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2] = lum, lum, lum
		}
	}
}

// feedFrames feeds frames to the BiteDetector at 50ms intervals and
// returns the index of the frame that triggered detection, or -1.
func feedFrames(bd *BiteDetector, frames []*image.RGBA) int {
	start := time.Now()
	for i, f := range frames {
		t := start.Add(time.Duration(i) * 50 * time.Millisecond)
		if bd.FeedFrame(f, t) {
			return i
		}
	}
	return -1
}

func TestBiteDetector_TriggersOnSyntheticSpike(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	base := byte(80)
	var frames []*image.RGBA
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, base, nil))
	}
	for i := 0; i < 2; i++ {
		frames = append(frames, synthFrame(w, h, base, func(px []byte, w, h int) { applyRegion(px, w, h, 10, 10, 30, 30, 140) }))
	}
	frames = append(frames, synthFrame(w, h, base, nil))
	idx := feedFrames(bd, frames)
	if idx < 0 {
		t.Fatalf("expected detection, got none")
	}
	if idx != 5 {
		t.Fatalf("expected trigger at frame 5, got %d", idx)
	}
	if bd.FeedFrame(synthFrame(w, h, base, nil), time.Now()) {
		t.Fatalf("unexpected retrigger")
	}
}

func TestBiteDetector_NoTriggerOnNoise(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	base := byte(80)
	for i := 0; i < 50; i++ {
		f := synthFrame(w, h, base, func(px []byte, w, h int) {
			delta := byte((i % 3) + 1)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if (x+y+i)%11 == 0 {
						idx := (y*w + x) * 4
						v := px[idx]
						if v+delta < 255 {
							v += delta
						}
						px[idx], px[idx+1], px[idx+2] = v, v, v
					}
				}
			}
		})
		if bd.FeedFrame(f, time.Now().Add(time.Duration(i)*50*time.Millisecond)) {
			t.Fatalf("noise trigger at frame %d", i)
		}
	}
}

func TestBiteDetector_NoTriggerOnSlowDrift(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	base := byte(80)
	for i := 0; i < 40; i++ {
		f := synthFrame(w, h, base+byte(i/8), nil)
		if bd.FeedFrame(f, time.Now().Add(time.Duration(i)*60*time.Millisecond)) {
			t.Fatalf("slow drift trigger at %d", i)
		}
	}
}

func TestBiteDetector_ResetClearsState(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	base := byte(80)
	frames := []*image.RGBA{}
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, base, nil))
	}
	for i := 0; i < 2; i++ {
		frames = append(frames, synthFrame(w, h, base, func(px []byte, w, h int) { applyRegion(px, w, h, 5, 5, 25, 25, 150) }))
	}
	if feedFrames(bd, frames) < 0 {
		t.Fatalf("expected first detection")
	}
	bd.Reset()
	frames = frames[:0]
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, base, nil))
	}
	for i := 0; i < 2; i++ {
		frames = append(frames, synthFrame(w, h, base, func(px []byte, w, h int) { applyRegion(px, w, h, 10, 10, 30, 30, 150) }))
	}
	if feedFrames(bd, frames) < 0 {
		t.Fatalf("expected second detection after reset")
	}
}
