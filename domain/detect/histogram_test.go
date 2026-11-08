package detect

import (
	"image"
	"testing"
	"time"
)

// feedHistogram sends calm frames every 100 ms up to the given time, then
// frame, and reports whether any frame fired.
func feedHistogram(h *HistogramBiteDetector, upTo time.Duration, frame *image.RGBA) bool {
	start := time.Unix(0, 0)
	fired := false
	for t := time.Duration(0); t < upTo; t += 100 * time.Millisecond {
		fired = h.Feed(water(0, 0, 0), start.Add(t)) || fired
	}
	return h.Feed(frame, start.Add(upTo)) || fired
}

func TestHistogram(t *testing.T) {
	tests := []struct {
		name  string
		at    time.Duration
		frame *image.RGBA
		want  bool
	}{
		{"splash covers the bobber", 4 * time.Second, water(50, 45, 30), true},
		{"calm water", 4 * time.Second, water(0, 0, 0), false},
		{"splash during the warm-up", time.Second, water(50, 45, 30), false},
		{"small glint", 4 * time.Second, water(50, 45, 8), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := feedHistogram(NewHistogramBiteDetector(nil), tt.at, tt.frame); got != tt.want {
				t.Errorf("fired = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHistogramFiresOnceUntilReset(t *testing.T) {
	h := NewHistogramBiteDetector(nil)
	if !feedHistogram(h, 4*time.Second, water(50, 45, 30)) {
		t.Fatal("no bite")
	}
	if h.Feed(water(50, 45, 30), time.Unix(5, 0)) {
		t.Error("fired twice")
	}
	h.Reset()
	if !feedHistogram(h, 4*time.Second, water(50, 45, 30)) {
		t.Error("no bite after Reset")
	}
}
