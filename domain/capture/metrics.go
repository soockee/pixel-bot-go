package capture

import (
	"image"
	"time"
)

// FrameSnapshot carries the latest captured frame and metadata.
type FrameSnapshot struct {
	Image      *image.RGBA
	CapturedAt time.Time
	Sequence   uint64
}

// CaptureStats summarises capture loop behaviour for instrumentation.
type CaptureStats struct {
	Captures         uint64
	Skipped          uint64
	AvgCapture       time.Duration
	AvgCaptureMicros float64
	LastCapture      time.Time
	LatestFrameAge   time.Duration
	Sequence         uint64
}
