package detect

import (
	"image"
	"image/color"
	"testing"
	"time"
)

// water returns a calm 100x90 frame with an optional bright disc at (x, y).
func water(x, y, radius int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 90))
	for py := range 90 {
		for px := range 100 {
			c := color.RGBA{50, 60, 70, 255}
			if (px-x)*(px-x)+(py-y)*(py-y) <= radius*radius {
				c = color.RGBA{180, 190, 200, 255}
			}
			img.SetRGBA(px, py, c)
		}
	}
	return img
}

// feed sends calm frames every 100 ms up to the given time, then frame, and
// reports whether any frame fired.
func feed(s *SplashBiteDetector, upTo time.Duration, frame *image.RGBA) bool {
	start := time.Unix(0, 0)
	fired := false
	for t := time.Duration(0); t < upTo; t += 100 * time.Millisecond {
		fired = s.Feed(water(0, 0, 0), start.Add(t)) || fired
	}
	return s.Feed(frame, start.Add(upTo)) || fired
}

func TestSplash(t *testing.T) {
	tests := []struct {
		name  string
		at    time.Duration
		frame *image.RGBA
		want  bool
	}{
		{"splash at the bobber", 4 * time.Second, water(50, 45, 10), true},
		{"calm water", 4 * time.Second, water(0, 0, 0), false},
		{"landing ripples before the scene settles", time.Second, water(50, 45, 10), false},
		{"effect away from the bobber", 4 * time.Second, water(10, 10, 10), false},
		{"effect across the whole box", 4 * time.Second, water(50, 45, 80), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := feed(NewSplashBiteDetector(nil), tt.at, tt.frame); got != tt.want {
				t.Errorf("fired = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplashFiresOnceUntilReset(t *testing.T) {
	s := NewSplashBiteDetector(nil)
	if !feed(s, 4*time.Second, water(50, 45, 10)) {
		t.Fatal("no bite")
	}
	if s.Feed(water(50, 45, 10), time.Unix(5, 0)) {
		t.Error("fired twice")
	}
	s.Reset()
	if !feed(s, 4*time.Second, water(50, 45, 10)) {
		t.Error("no bite after Reset")
	}
}
