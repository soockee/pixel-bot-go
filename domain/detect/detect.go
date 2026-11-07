// Package detect finds the bobber in a frame and recognises bites.
package detect

import (
	"image"
	"time"
)

// Match is a located bobber. X and Y are its centre in frame coordinates.
type Match struct {
	X, Y  int
	Score float64
	Scale float64
}

// Locator finds the bobber. The bool reports a usable match; the Match holds
// the best candidate either way, for logging.
type Locator interface {
	Locate(frame *image.RGBA) (Match, bool)
}

// BiteDetector watches the region around a locked bobber. Call Reset when a new
// cast starts, and Feed from one goroutine.
type BiteDetector interface {
	Feed(roi *image.RGBA, t time.Time) bool
	Reset()
}
