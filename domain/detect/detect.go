// Package detect finds the bobber in a frame and recognises bites.
package detect

import (
	"image"
	"time"
)

// Match is a located bobber. X and Y are the click point and Box is the area to
// watch for a bite, both in frame coordinates.
type Match struct {
	X, Y  int
	Box   image.Rectangle
	Score float64
	Scale float64
}

// Locator finds the bobber. The bool reports a usable match; the Match holds
// the best candidate either way, for logging.
type Locator interface {
	Locate(frame *image.RGBA) (Match, bool)
}

// CastLocator starts a fresh search after a cast. Its first frame should show
// the scene before the bobber lands.
type CastLocator interface {
	Locator
	BeginCast()
}

// BiteDetector watches the region around a locked bobber. Call Reset when a new
// cast starts, and Feed from one goroutine.
type BiteDetector interface {
	Feed(roi *image.RGBA, t time.Time) bool
	Reset()
}

// SoundBiteDetector is a BiteDetector that also listens to the game. Hear takes
// each packet of capture.CaptureAudio in play order, on the goroutine that calls
// Feed, and reports a bite.
type SoundBiteDetector interface {
	BiteDetector
	Hear(pcm []byte, t time.Time) bool
}
