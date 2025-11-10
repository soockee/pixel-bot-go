package capture

import (
	"image"
	"log/slog"
	"sync/atomic"
	"time"
)

const captureStatsLogInterval = 5 * time.Second

// CaptureService acquires image frames (selection or full screen) and exposes the
// latest capture alongside instrumentation data. Use NewCaptureService to
// construct an instance.
type CaptureService interface {
	Start()
	Stop()
	LatestFrame() FrameSnapshot
	Running() bool
	SetSelectionProvider(func() *image.Rectangle)
	Stats() CaptureStats
}

type captureService struct {
	running      atomic.Bool
	latest       atomic.Pointer[FrameSnapshot]
	selFn        func() *image.Rectangle // user selection rectangle (optional)
	logger       *slog.Logger
	captures     atomic.Uint64
	skipped      atomic.Uint64
	captureNanos atomic.Uint64
	sequence     atomic.Uint64
}

func newCaptureService(logger *slog.Logger, selectionFn func() *image.Rectangle) *captureService {
	return &captureService{selFn: selectionFn, logger: logger}
}

// NewCaptureService constructs a capture service that provides frames via Frames().
func NewCaptureService(logger *slog.Logger, selectionFn func() *image.Rectangle) CaptureService {
	return newCaptureService(logger, selectionFn)
}

func (s *captureService) SetSelectionProvider(fn func() *image.Rectangle) { s.selFn = fn }

func (s *captureService) LatestFrame() FrameSnapshot {
	snap := s.latest.Load()
	if snap == nil {
		return FrameSnapshot{}
	}
	return *snap
}

func (s *captureService) Running() bool { return s.running.Load() }

func (s *captureService) Stats() CaptureStats {
	captures := s.captures.Load()
	skipped := s.skipped.Load()
	total := s.captureNanos.Load()
	var avg time.Duration
	avgMicros := 0.0
	if captures > 0 && total > 0 {
		avg = time.Duration(total / captures)
		avgMicros = float64(avg) / float64(time.Microsecond)
	}
	snapshot := s.LatestFrame()
	age := time.Duration(0)
	if !snapshot.CapturedAt.IsZero() {
		age = time.Since(snapshot.CapturedAt)
	}
	return CaptureStats{
		Captures:         captures,
		Skipped:          skipped,
		AvgCapture:       avg,
		AvgCaptureMicros: avgMicros,
		LastCapture:      snapshot.CapturedAt,
		LatestFrameAge:   age,
		Sequence:         snapshot.Sequence,
	}
}

func (s *captureService) Start() {
	if s.running.Load() {
		return
	}
	s.running.Store(true)
	go s.loop()
}

func (s *captureService) Stop() {
	if !s.running.Load() {
		return
	}
	s.running.Store(false)
}

func (s *captureService) loop() {
	logTicker := time.NewTicker(captureStatsLogInterval)
	defer logTicker.Stop()
	for s.running.Load() {
		start := time.Now()
		var img *image.RGBA

		if s.selFn != nil {
			if r := s.selFn(); r != nil && !r.Empty() {
				if out, err := GrabSelection(*r); err == nil {
					img = out
				} else if s.logger != nil {
					s.logger.Error("capture selection", "error", err)
				}
			}
		}

		if img == nil {
			if full, err := Grab(); err != nil {
				if s.logger != nil {
					s.logger.Error("capture full", "error", err)
				}
			} else if full != nil {
				img = full
			}
		}

		if img == nil {
			s.skipped.Add(1)
			time.Sleep(1 * time.Millisecond)
			continue
		}

		elapsed := time.Since(start)
		s.captureNanos.Add(uint64(elapsed.Nanoseconds()))
		s.captures.Add(1)
		seq := s.sequence.Add(1)
		s.latest.Store(&FrameSnapshot{Image: img, CapturedAt: time.Now(), Sequence: seq})

		select {
		case <-logTicker.C:
			s.logStats()
		default:
		}

		time.Sleep(200 * time.Microsecond)
	}
}

func (s *captureService) logStats() {
	if s.logger == nil {
		return
	}
	stats := s.Stats()
	s.logger.Debug("capture.stats",
		"captures", stats.Captures,
		"skipped", stats.Skipped,
		"avg_capture", stats.AvgCapture,
		"age", stats.LatestFrameAge,
	)
}
