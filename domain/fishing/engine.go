package fishing

import (
	"context"
	"errors"
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
	// staleFor is how long after a cast a marker where the last bobber was is
	// taken for that bobber, which the game is still fading out.
	staleFor = 3 * time.Second
)

// markerIsStale reports whether a marker found since a cast (the time since
// the search began) is the last bobber's, not a new one: it sits on the last
// bobber's box, the fade is not over, and the marker has not been missing
// since the cast. A new bobber at the same spot comes after a gap, because the
// old one vanishes before the new one lands.
func markerIsStale(box, last image.Rectangle, since time.Duration, gone bool) bool {
	if last.Empty() || gone || since >= staleFor {
		return false
	}
	overlap := box.Intersect(last)
	return overlap.Dx()*overlap.Dy()*4 > last.Dx()*last.Dy() // more than a quarter of the old box
}

// FrameSource captures the game. Window is the game window's client area in
// screen pixels. Grab captures an area of the screen. Both methods must be safe
// for concurrent calls to confirm the target before input.
type FrameSource interface {
	Window() (image.Rectangle, error)
	Grab(area image.Rectangle) (capture.Frame, error)
}

// SoundSource streams the game's sound to emit until ctx ends. The first sample
// of each packet played at the given time.
type SoundSource func(ctx context.Context, emit func(pcm []byte, at time.Time) error) error

// soundPacket is one packet from a SoundSource.
type soundPacket struct {
	pcm []byte
	at  time.Time
}

// Preview is the latest image for display. While a bobber is watched only its
// box is captured, so Frame is the last view of the whole window and ROI, at
// ROIBox in Frame's pixels, is live. ROI is nil outside monitoring.
type Preview struct {
	Frame  *image.RGBA
	ROI    *image.RGBA
	ROIBox image.Rectangle
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
	cfg := *e.cfg
	if err := CheckActivityKeys(cfg, e.fsm.actions.ParseVK); err != nil {
		return err
	}
	switch cfg.ReelMethod {
	case "", config.ReelRightClick:
	case config.ReelInteractKey:
		if cfg.Locator != "marker" {
			return errors.New("Interact key requires the marker locator")
		}
		if _, ok := e.fsm.interactVK(cfg); !ok {
			return errors.New("set an Interact key that differs from the Cast and Stop keys")
		}
	default:
		return errors.New("unknown reel method")
	}
	locator, bite, err := e.detectors(cfg)
	if err != nil {
		return err
	}
	var closeKeyboard func()
	if cfg.ESP32Port != "" {
		if !cfg.DryRun {
			if e.fsm.actions.OpenKeyboard == nil || e.fsm.actions.TapKeyboard == nil {
				return errors.New("ESP32 keyboard backend unavailable")
			}
			closeKeyboard, err = e.fsm.actions.OpenKeyboard(cfg)
			if err != nil {
				return err
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel, e.done = cancel, make(chan struct{})
	r := &run{engine: e, locator: locator, bite: bite, interact: cfg.ReelMethod == config.ReelInteractKey, watchPlayer: cfg.TargetPlayerKey != ""}
	r.listener, _ = bite.(detect.SoundBiteDetector)
	go r.loop(ctx, e.done, closeKeyboard)
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
	engine                 *Engine
	locator                detect.Locator
	bite                   detect.BiteDetector
	listener               detect.SoundBiteDetector // bite, when it also listens
	target                 image.Rectangle          // locked area in screen coordinates; the last bobber's while searching
	targetWindow           image.Rectangle          // client area when the target was found
	searchStart            time.Time                // when this search began, zero outside searching
	markerGone             bool                     // no marker has been seen since this search began
	window                 capture.Frame            // the last capture of the whole game window
	lastSearch             time.Time
	searches               int
	monitoring             bool
	interact               bool
	lastErr                string
	watchPlayer            bool
	playerPoll             time.Time
	playerSeen, playerSent uint8
}

func (r *run) loop(ctx context.Context, done chan<- struct{}, closeKeyboard func()) {
	defer close(done)
	if closeKeyboard != nil {
		defer closeKeyboard()
	}
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

// step captures and runs the detection the FSM state calls for. A search
// needs the whole game window; watching a bobber needs only its box, which
// captures as fast as any small area, so a bite is seen as soon as before.
func (r *run) step() {
	e := r.engine
	state := e.fsm.Current()
	if state != StateSearching {
		r.searchStart = time.Time{}
	} else if r.searchStart.IsZero() {
		r.searchStart, r.markerGone = time.Now(), false
	}
	watching := state == StateMonitoring && !r.target.Empty() && r.window.Image != nil
	window, err := e.frames.Window()
	if (watching || state == StateReeling && !r.target.Empty()) && (err != nil || window != r.targetWindow) {
		r.monitoring = false
		e.preview.Store(&Preview{Frame: r.window.Image})
		e.fsm.send(evtTargetInvalidated{})
		return
	}
	area := window
	if watching {
		area = r.target
	}
	var frame capture.Frame
	if err == nil {
		frame, err = e.frames.Grab(area)
	}
	if err != nil {
		if err.Error() != r.lastErr { // a failure can last for thousands of ticks
			e.logger.Error("capture failed", "error", err)
			r.lastErr = err.Error()
		}
		return
	}
	r.lastErr = ""
	if !watching {
		r.window = frame
		r.monitoring = false
		if state == StateSearching {
			r.search(frame, window)
		}
		e.preview.Store(&Preview{Frame: frame.Image})
		return
	}
	roi := r.monitor(frame)
	if r.watchPlayer && e.fsm.Current() == StateMonitoring {
		r.pollPlayer(window)
	}
	e.preview.Store(&Preview{Frame: r.window.Image, ROI: roi, ROIBox: r.target.Sub(r.window.Origin)})
}

func (r *run) search(frame capture.Frame, window image.Rectangle) {
	if time.Since(r.lastSearch) < searchInterval {
		return
	}
	r.lastSearch = time.Now()
	e := r.engine
	match, found := r.locator.Locate(frame.Image)
	r.searches++
	if !found {
		r.markerGone = true
	} else if markerIsStale(match.Box.Add(frame.Origin), r.target, time.Since(r.searchStart), r.markerGone) {
		e.logger.Debug("detection.search ignored the last bobber's marker", "x", match.X, "y", match.Y)
		found = false
	}
	if found || r.searches%10 == 0 {
		e.logger.Debug("detection.search", "found", found, "score", match.Score, "scale", match.Scale, "x", match.X, "y", match.Y,
			"frame_w", frame.Image.Bounds().Dx(), "frame_h", frame.Image.Bounds().Dy(), "dur_ms", time.Since(r.lastSearch).Milliseconds())
	}
	if found {
		r.target = match.Box.Add(frame.Origin)
		r.targetWindow = window
		target := evtTargetAcquiredAt{
			x: frame.Origin.X + match.X, y: frame.Origin.Y + match.Y, box: r.target,
			valid: func() bool {
				current, err := e.frames.Window()
				return err == nil && current == window
			},
		}
		// A miss may click just outside the marker, away from the window edge.
		for _, x := range []int{r.target.Min.X - 4, r.target.Max.X + 3} {
			aim := image.Pt(x, target.y)
			if aim.In(window.Inset(8)) && !aim.In(r.target) {
				target.missAim = &aim
				break
			}
		}
		if r.interact {
			area := r.target.Inset(-8).Intersect(window)
			point := image.Pt(target.x, target.y)
			target.confirm = func() bool {
				fresh, err := e.frames.Grab(area)
				if err != nil || fresh.Image == nil {
					return false
				}
				marker, found := detect.NewMarkerLocator().Locate(fresh.Image)
				return found && point.In(marker.Box.Add(fresh.Origin))
			}
		}
		e.fsm.send(target)
	}
}

func (r *run) monitor(frame capture.Frame) *image.RGBA {
	e := r.engine
	box := r.target.Sub(frame.Origin)
	if !box.In(frame.Image.Bounds()) {
		r.monitoring = false
		return nil
	}
	if !r.monitoring {
		r.bite.Reset()
		r.monitoring = true
	}
	roi := frame.Image.SubImage(box).(*image.RGBA)
	if r.bite.Feed(roi, frame.CapturedAt) {
		e.fsm.EventFishBite()
	}
	return roi
}
