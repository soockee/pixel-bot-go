package fishing

import (
	"context"
	"image"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
	"github.com/soocke/pixel-bot-go/domain/detect"
)

const (
	frameInterval  = 33 * time.Millisecond
	searchInterval = 65 * time.Millisecond // a search can use every core, so leave gaps
)

// FrameSource delivers the current image of the watched region.
type FrameSource interface {
	Next() (capture.Frame, error)
}

// Preview is the latest image for display. ROI is nil outside monitoring.
type Preview struct {
	Frame *image.RGBA
	ROI   *image.RGBA
}

// Engine runs the pipeline on one goroutine: it grabs a frame, runs the
// detection the FSM state calls for, and feeds the result to the FSM. It has no
// UI dependency. Start and Stop must be called from one goroutine.
type Engine struct {
	frames    FrameSource
	fsm       *FishingFSM
	detectors DetectorFactory
	cfg       *config.Config
	logger    *slog.Logger
	preview   atomic.Pointer[Preview]
	cancel    context.CancelFunc
	done      chan struct{}
}

// DetectorFactory builds the detectors for a run from that run's config.
type DetectorFactory func(config.Config) (detect.Locator, detect.BiteDetector, error)

// NewEngine returns a stopped Engine.
func NewEngine(frames FrameSource, fsm *FishingFSM, detectors DetectorFactory, cfg *config.Config, logger *slog.Logger) *Engine {
	return &Engine{frames: frames, fsm: fsm, detectors: detectors, cfg: cfg, logger: logger}
}

// Start begins a run with the detectors the current config names. Starting twice
// is a no-op.
func (e *Engine) Start() error {
	if e.cancel != nil {
		return nil
	}
	locator, bite, err := e.detectors(*e.cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel, e.done = cancel, make(chan struct{})
	r := &run{engine: e, locator: locator, bite: bite}
	go r.loop(ctx, e.done)
	return nil
}

// Stop ends the run, waits for its goroutine and clears the preview.
func (e *Engine) Stop() {
	if e.cancel == nil {
		return
	}
	e.cancel()
	<-e.done
	e.cancel = nil
	e.preview.Store(nil)
}

// Preview returns the latest images, or nil before the first frame of a run.
func (e *Engine) Preview() *Preview { return e.preview.Load() }

// run holds the state of one Start to Stop span. Only its goroutine touches it.
type run struct {
	engine     *Engine
	locator    detect.Locator
	bite       detect.BiteDetector
	target     image.Rectangle // locked area in screen coordinates
	lastSearch time.Time
	searches   int
	monitoring bool
	lastErr    string
}

func (r *run) loop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.step()
		}
	}
}

func (r *run) step() {
	e := r.engine
	frame, err := e.frames.Next()
	if err != nil {
		if err.Error() != r.lastErr { // a failure can last for thousands of ticks
			e.logger.Error("capture failed", "error", err)
			r.lastErr = err.Error()
		}
		return
	}
	r.lastErr = ""
	preview := &Preview{Frame: frame.Image}
	switch e.fsm.Current() {
	case StateSearching:
		r.monitoring = false
		r.search(frame)
	case StateMonitoring:
		preview.ROI = r.monitor(frame)
	}
	e.preview.Store(preview)
}

func (r *run) search(frame capture.Frame) {
	if time.Since(r.lastSearch) < searchInterval {
		return
	}
	r.lastSearch = time.Now()
	e := r.engine
	match, found := r.locator.Locate(frame.Image)
	r.searches++
	if found || r.searches%10 == 0 {
		e.logger.Debug("detection.search", "found", found, "score", match.Score, "scale", match.Scale, "x", match.X, "y", match.Y,
			"frame_w", frame.Image.Bounds().Dx(), "frame_h", frame.Image.Bounds().Dy(), "dur_ms", time.Since(r.lastSearch).Milliseconds())
	}
	if found {
		r.target = match.Box.Add(frame.Origin)
		e.fsm.EventTargetAcquiredAt(frame.Origin.X+match.X, frame.Origin.Y+match.Y)
	}
}

func (r *run) monitor(frame capture.Frame) *image.RGBA {
	e := r.engine
	if !r.monitoring {
		r.bite.Reset()
		r.monitoring = true
	}
	roi := frame.Image.SubImage(r.target.Sub(frame.Origin)).(*image.RGBA)
	if r.bite.Feed(roi, frame.CapturedAt) {
		e.fsm.EventFishBite()
	}
	return roi
}
