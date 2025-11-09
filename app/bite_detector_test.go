package app

import (
	"image"
	"testing"
	"time"
)

// synthFrame creates an RGBA frame of size w x h with a uniform base luminance
// (applied equally to R,G,B) and then lets a mutate function modify pixels.
func synthFrame(w, h int, base byte, mutate func(px []byte, w, h int)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			img.Pix[i] = base
			img.Pix[i+1] = base
			img.Pix[i+2] = base
			img.Pix[i+3] = 255
		}
	}
	if mutate != nil {
		mutate(img.Pix, w, h)
	}
	return img
}

// applyRegion sets a rectangular region to a given luminance value.
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
			px[i] = lum
			px[i+1] = lum
			px[i+2] = lum
		}
	}
}

// feedFrames feeds frames with timestamps spaced by ~50ms (20fps approximation).
func feedFrames(bd *BiteDetector, frames []*image.RGBA) (triggerIndex int) {
	start := time.Now()
	for i, f := range frames {
		t := start.Add(time.Duration(i) * 50 * time.Millisecond)
		if bd.FeedFrame(f, t) {
			return i
		}
	}
	return -1
}

// Test that a strong localized spike triggers detection.
func TestBiteDetector_TriggersOnSyntheticSpike(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	// Use a moderate frame size; changed region will exceed ratio thresholds.
	w, h := 40, 40
	baseLum := byte(80)
	var frames []*image.RGBA
	// Initial stable frames.
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, baseLum, nil))
	}
	// Two consecutive spike frames (region 20x20 -> 400/1600 = 0.25 > ratioThresholdSpike 0.18)
	for i := 0; i < 2; i++ {
		frames = append(frames, synthFrame(w, h, baseLum, func(px []byte, w, h int) {
			applyRegion(px, w, h, 10, 10, 30, 30, 140) // +60 luminance
		}))
	}
	// Post-spike calm frame
	frames = append(frames, synthFrame(w, h, baseLum, nil))

	idx := feedFrames(bd, frames)
	if idx < 0 {
		t.Fatalf("expected detection, got none")
	}
	// Detection should occur on second spike frame given debounce=2
	// Spike frames start at index 5 and 6.
	// With single-frame debounce, trigger should occur on first spike frame (index 5)
	if idx != 5 {
		t.Fatalf("expected trigger at frame 5, got %d", idx)
	}
	// Subsequent frames should not retrigger.
	if bd.FeedFrame(synthFrame(w, h, baseLum, nil), time.Now()) {
		t.Fatalf("unexpected retrigger after initial detection")
	}
}

// Test that random small noise does not trigger.
func TestBiteDetector_NoTriggerOnNoise(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	baseLum := byte(80)
	noiseFrames := 50
	for i := 0; i < noiseFrames; i++ {
		f := synthFrame(w, h, baseLum, func(px []byte, w, h int) {
			// Apply small +/-5 luminance jitter below pixelDiffThreshold (10)
			// Keep deterministic pattern for test stability.
			delta := byte((i % 3) + 1) // 1..3
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if (x+y+i)%11 == 0 { // sparse noise
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
			t.Fatalf("noise should not trigger detection (frame %d)", i)
		}
	}
}

// Test that a slow global drift does not trigger.
func TestBiteDetector_NoTriggerOnSlowDrift(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	baseLum := byte(80)
	for i := 0; i < 40; i++ {
		f := synthFrame(w, h, baseLum+byte(i/8), nil) // increase every 8 frames by +1
		if bd.FeedFrame(f, time.Now().Add(time.Duration(i)*60*time.Millisecond)) {
			t.Fatalf("slow drift should not trigger (frame %d)", i)
		}
	}
}

// Test that Reset fully clears state allowing a second detection.
func TestBiteDetector_ResetClearsState(t *testing.T) {
	bd := NewBiteDetector(nil, nil)
	bd.Reset()
	w, h := 40, 40
	baseLum := byte(80)
	// First detection sequence
	frames := []*image.RGBA{}
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, baseLum, nil))
	}
	for i := 0; i < 2; i++ { // spike
		frames = append(frames, synthFrame(w, h, baseLum, func(px []byte, w, h int) {
			applyRegion(px, w, h, 5, 5, 25, 25, 150)
		}))
	}
	if idx := feedFrames(bd, frames); idx < 0 {
		t.Fatalf("expected first detection, got none")
	}
	// Reset and attempt second detection
	bd.Reset()
	frames = frames[:0]
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, baseLum, nil))
	}
	for i := 0; i < 2; i++ { // spike again
		frames = append(frames, synthFrame(w, h, baseLum, func(px []byte, w, h int) {
			applyRegion(px, w, h, 10, 10, 30, 30, 150)
		}))
	}
	if idx := feedFrames(bd, frames); idx < 0 {
		t.Fatalf("expected second detection after reset, got none")
	}
}
