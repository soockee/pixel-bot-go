package capture

import (
	"image"
	"log/slog"
	"sync/atomic"
	"time"
)

// CaptureService acquires frames (either full screen or a selection rectangle) and publishes them.
// Start launches a goroutine; frames can be read from Frames(). Stop signals termination.
// Zero value is not usable; construct with newCaptureService.
type CaptureService interface {
	Start()
	Stop()
	Frames() <-chan *image.RGBA
	Running() bool
	SetSelectionProvider(func() *image.Rectangle)
}

type captureService struct {
	running atomic.Bool
	frameCh chan *image.RGBA
	selFn   func() *image.Rectangle
	logger  *slog.Logger
	// instrumentation counters (unexported)
	captures     atomic.Uint64 // successful captures delivered
	skipped      atomic.Uint64 // iterations skipped due to full buffer
	errors       atomic.Uint64 // capture attempts that returned an error or nil image
	captureNanos atomic.Uint64 // total nanoseconds spent capturing (successful only)
	// error logging control
}

func newCaptureService(logger *slog.Logger, selectionFn func() *image.Rectangle) *captureService {
	cs := &captureService{frameCh: make(chan *image.RGBA, 1), selFn: selectionFn, logger: logger}
	return cs
}

// NewCaptureService exports a constructor returning the public interface, allowing
// composition roots (container) to depend only on the interface without touching
// the concrete struct.
func NewCaptureService(logger *slog.Logger, selectionFn func() *image.Rectangle) CaptureService {
	// Legacy bus parameter ignored; global bus used for all events.
	return newCaptureService(logger, selectionFn)
}

func (s *captureService) SetSelectionProvider(fn func() *image.Rectangle) { s.selFn = fn }

func (s *captureService) Frames() <-chan *image.RGBA { return s.frameCh }

func (s *captureService) Running() bool { return s.running.Load() }

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
	// Drain one frame to avoid stale reference retention.
	select {
	case <-s.frameCh:
	default:
	}
}

func (s *captureService) loop() {
	// We only capture when the channel is empty to avoid doing work for frames that would be dropped.
	// Yield or microsleep when full to reduce CPU pressure under slow consumers.
	for s.running.Load() {
		if len(s.frameCh) == cap(s.frameCh) {
			// Buffer full; skip capture to avoid wasted allocations; micro-sleep reduces CPU without large latency penalty.
			s.skipped.Add(1)
			time.Sleep(200 * time.Microsecond)
			continue
		}
		start := time.Now()
		var img *image.RGBA
		var err error
		// Try selection first if provided.
		if sel := s.selFn; sel != nil {
			if r := sel(); r != nil {
				img, err = GrabSelection(*r)
				if err != nil {
					s.logger.Error("capture loop", "error", err)
				}
			}
		}
		// Fallback to full screen if selection failed or not set.
		if img == nil {
			full, err := Grab()
			if err != nil {
				s.logger.Error("capture loop", "error", err)
			} else if full != nil {
				img = full
			}
		}
		if img == nil {
			panic(1)
		}
		elapsed := time.Since(start)
		s.captureNanos.Add(uint64(elapsed.Nanoseconds()))
		s.captures.Add(1)
		// Blocking send is safe: we checked channel was empty.
		s.frameCh <- img
	}
}
