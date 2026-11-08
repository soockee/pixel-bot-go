package fishing

import (
	"context"
	"image"
	"log/slog"
	"sync"
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

// SoundSource streams the game's sound to emit until ctx ends. The first sample
// of each packet played at the given time.
type SoundSource func(ctx context.Context, emit func(pcm []byte, at time.Time) error) error

// soundPacket is one packet from a SoundSource.
type soundPacket struct {
	pcm []byte
	at  time.Time
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
	sound     SoundSource
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

// NewEngine returns a stopped Engine. It listens to sound only for a bite
// detector that implements detect.SoundBiteDetector.
func NewEngine(frames FrameSource, sound SoundSource, fsm *FishingFSM, detectors DetectorFactory, cfg *config.Config, logger *slog.Logger) *Engine {
	return &Engine{frames: frames, sound: sound, fsm: fsm, detectors: detectors, cfg: cfg, logger: logger}
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
	r.listener, _ = bite.(detect.SoundBiteDetector)
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
	listener   detect.SoundBiteDetector // bite, when it also listens
	target     image.Rectangle          // locked area in screen coordinates
	lastSearch time.Time
	searches   int
	monitoring bool
	lastErr    string
}

func (r *run) loop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	var listening sync.WaitGroup
	defer listening.Wait()
	sounds := r.listen(ctx, &listening)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.step()
		case p := <-sounds:
			r.hear(p)
		}
	}
}

// listen streams sound into the returned channel while the run lasts, or
// returns nil when the bite detector does not listen.
func (r *run) listen(ctx context.Context, wg *sync.WaitGroup) <-chan soundPacket {
	if r.listener == nil {
		return nil
	}
	sounds := make(chan soundPacket, 256) // 2.5 s of 10 ms packets
	e := r.engine
	wg.Go(func() {
		err := e.sound(ctx, func(pcm []byte, at time.Time) error {
			select {
			case sounds <- soundPacket{pcm, at}:
			default: // the loop is busy; a dropped packet only blurs one level
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			e.logger.Error("sound capture failed; bites cannot be heard", "error", err)
		}
	})
	return sounds
}

// hear gives a sound packet to the bite detector while it watches a bobber.
func (r *run) hear(p soundPacket) {
	if !r.monitoring || r.engine.fsm.Current() != StateMonitoring {
		return
	}
	if r.listener.Hear(p.pcm, p.at) {
		r.engine.fsm.EventFishBite()
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
