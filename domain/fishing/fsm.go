package fishing

import (
	"image"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// FishingFSM manages fishing state, timers, detectors and side-effect actions.
// It runs an internal event loop on a goroutine and serializes state transitions.
type FishingFSM struct {
	state            FishingState
	logger           *slog.Logger
	cfg              *config.Config
	cooldownDuration time.Duration
	cooldownUntil    time.Time
	searchTimer      *time.Timer
	cooldownTimer    *time.Timer
	coordX, coordY   int
	coordSet         bool
	biteDetector     BiteDetectorContract
	closed           bool
	actions          ActionCallbacks
	detectorCtor     DetectorFactory
	events           chan interface{}
	listeners        []FishingStateListener
}

// NewFSM creates and starts a FishingFSM. The FSM starts in StateHalt.
// If cfg is nil a default cooldown is used.
func NewFSM(logger *slog.Logger, cfg *config.Config, actions ActionCallbacks, detectorCtor DetectorFactory) *FishingFSM {
	cooldown := time.Second
	if cfg != nil && cfg.CooldownSeconds > 0 {
		cooldown = time.Duration(cfg.CooldownSeconds) * time.Second
	}
	f := &FishingFSM{state: StateHalt, logger: logger, cfg: cfg, cooldownDuration: cooldown, events: make(chan interface{}, 64), actions: actions, detectorCtor: detectorCtor}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				if logger != nil {
					logger.Error("fsm panic", "error", r, "stack", stack)
				}
			}
		}()
		f.loop()
	}()
	return f
}

func (f *FishingFSM) loop() {
	for ev := range f.events {
		switch e := ev.(type) {
		case FishingStateListener: // unlikely direct send, ignore
		case evtAddListener:
			f.listeners = append(f.listeners, e.l)
		case evtTargetAcquired:
			if f.state == StateSearching {
				f.transition(StateMonitoring)
			}
		case evtTargetAcquiredAt:
			f.coordX, f.coordY, f.coordSet = e.x, e.y, true
			if f.state == StateSearching {
				f.transition(StateMonitoring)
			}
		case evtTargetLost:
			if f.state == StateMonitoring {
				f.transition(StateCasting)
			}
		case evtHalt:
			f.cooldownUntil = time.Time{}
			f.coordSet = false
			if f.biteDetector != nil {
				f.biteDetector.Reset()
			}
			f.transition(StateHalt)
		case evtFishBite:
			if f.state == StateMonitoring {
				f.transition(StateReeling)
			}
		case evtMonitoringFrame:
			if f.state == StateMonitoring && f.biteDetector != nil && e.roi != nil {
				if f.biteDetector.FeedFrame(e.roi, e.now) {
					f.transition(StateReeling)
				} else if f.biteDetector.TargetLostHeuristic() {
					f.transition(StateCasting)
				}
			}
		case evtFocusAcquired:
			if f.state == StateWaitingFocus {
				f.transition(StateSearching)
			}
		case evtAwaitFocus:
			if f.state == StateHalt {
				f.transition(StateWaitingFocus)
			}
		case evtForceCast:
			if f.state != StateCasting {
				f.transition(StateCasting)
			}
		case evtCancel:
			f.cooldownUntil = time.Time{}
		}
	}
	f.closed = true
}

// internal event types sent to the FSM loop
type (
	evtTargetAcquired   struct{}
	evtTargetAcquiredAt struct{ x, y int }
	evtTargetLost       struct{}
	evtHalt             struct{}
	evtFishBite         struct{}
	evtFocusAcquired    struct{}
	evtAwaitFocus       struct{}
	evtForceCast        struct{}
	evtAddListener      struct{ l FishingStateListener }
	evtCancel           struct{}
	evtMonitoringFrame  struct {
		roi *image.RGBA
		now time.Time
	}
)

func (f *FishingFSM) transition(next FishingState) {
	prev := f.state
	if prev == next {
		return
	}
	// stop search timer when leaving StateSearching
	if prev == StateSearching && next != StateSearching && f.searchTimer != nil {
		f.searchTimer.Stop()
		f.searchTimer = nil
	}
	// stop cooldown timer when leaving StateCooldown
	if prev == StateCooldown && next != StateCooldown && f.cooldownTimer != nil {
		f.cooldownTimer.Stop()
		f.cooldownTimer = nil
	}
	switch next {
	case StateCasting:
		if f.cfg != nil && f.actions.PressKey != nil && f.actions.ParseVK != nil {
			vk := f.actions.ParseVK(f.cfg.ReelKey)
			go func() { defer recoverLog(f.logger, "cast goroutine panic"); f.actions.PressKey(vk) }()
			if f.logger != nil {
				f.logger.Info("cast action executed", "key", f.cfg.ReelKey)
			}
		}
	case StateReeling:
		if f.coordSet {
			cx, cy := f.coordX, f.coordY
			go func(x, y int) {
				defer recoverLog(f.logger, "reel goroutine panic")
				if f.actions.MoveCursor != nil {
					f.actions.MoveCursor(x, y)
				}
				time.Sleep(300 * time.Millisecond)
				if f.actions.ClickRight != nil {
					f.actions.ClickRight()
				}
				if f.logger != nil {
					f.logger.Info("reel action executed", "x", x, "y", y)
				}
			}(cx, cy)
		} else if f.logger != nil {
			f.logger.Info("reel action skipped - no target coords")
		}
		f.cooldownUntil = time.Now().Add(f.cooldownDuration + 500*time.Millisecond)
		next = StateCooldown
		// schedule cooldown timer (transition will not hit StateCooldown case after modifying next)
		if f.cooldownTimer != nil {
			f.cooldownTimer.Stop()
		}
		until := f.cooldownUntil
		f.cooldownTimer = time.AfterFunc(time.Until(until), func() {
			if f.state == StateCooldown && !f.closed {
				select {
				case f.events <- evtForceCast{}:
				default:
					if f.logger != nil {
						f.logger.Debug("force cast event (cooldown) dropped (channel full)")
					}
				}
			}
		})
	case StateCooldown:
		if f.cooldownUntil.IsZero() {
			f.cooldownUntil = time.Now().Add(f.cooldownDuration)
		}
		// start / restart cooldown timer
		if f.cooldownTimer != nil {
			f.cooldownTimer.Stop()
		}
		until := f.cooldownUntil
		f.cooldownTimer = time.AfterFunc(time.Until(until), func() {
			if f.state == StateCooldown && !f.closed {
				select {
				case f.events <- evtForceCast{}:
				default:
					if f.logger != nil {
						f.logger.Debug("force cast event (cooldown) dropped (channel full)")
					}
				}
			}
		})
	case StateMonitoring:
		if f.coordSet && f.actions.MoveCursor != nil {
			cx, cy := f.coordX, f.coordY
			go func(x, y int) {
				if f.actions.MoveCursor != nil {
					f.actions.MoveCursor(x, y)
				}
				if f.logger != nil {
					f.logger.Info("found blobber", "x", x, "y", y)
				}
			}(cx, cy)
		}
		if f.detectorCtor != nil {
			f.biteDetector = f.detectorCtor(nil, f.logger)
		} else {
			f.biteDetector = NewBiteDetector(nil, f.logger)
		}
		if f.biteDetector != nil {
			f.biteDetector.Reset()
		}
	case StateHalt: // no-op
	}
	f.state = next
	if f.state == StateSearching {
		// start / restart search timer (force cast after 5s)
		if f.searchTimer != nil {
			f.searchTimer.Stop()
		}
		f.searchTimer = time.AfterFunc(5*time.Second, func() {
			// only emit if still searching and not closed
			if f.state == StateSearching && !f.closed {
				select {
				case f.events <- evtForceCast{}:
				default:
					if f.logger != nil {
						f.logger.Debug("force cast event dropped (channel full)")
					}
				}
			}
		})
	}
	if f.logger != nil {
		f.logger.Debug("fishing state transition", "from", prev.String(), "to", next.String())
	}
	for _, l := range f.listeners {
		l(prev, next)
	}
	// if casting, immediately transition to searching to resume scan cycle
	if f.state == StateCasting {
		f.transition(StateSearching)
	}
}

// Tick is retained for backward compatibility; timers drive transitions.

// Public API methods
func (f *FishingFSM) AddListener(l FishingStateListener) { f.events <- evtAddListener{l: l} }
func (f *FishingFSM) Current() FishingState              { return f.state }
func (f *FishingFSM) EventTargetAcquired()               { f.events <- evtTargetAcquired{} }
func (f *FishingFSM) EventTargetAcquiredAt(x, y int)     { f.events <- evtTargetAcquiredAt{x: x, y: y} }
func (f *FishingFSM) EventTargetLost()                   { f.events <- evtTargetLost{} }
func (f *FishingFSM) EventHalt()                         { f.events <- evtHalt{} }
func (f *FishingFSM) EventFishBite()                     { f.events <- evtFishBite{} }
func (f *FishingFSM) EventFocusAcquired()                { f.events <- evtFocusAcquired{} }
func (f *FishingFSM) EventAwaitFocus()                   { f.events <- evtAwaitFocus{} }
func (f *FishingFSM) ForceCast()                         { f.events <- evtForceCast{} }
func (f *FishingFSM) Cancel()                            { f.events <- evtCancel{} }

// Tick is deprecated and is a no-op (retained for backward compatibility).
func (f *FishingFSM) Tick(now time.Time) {}
func (f *FishingFSM) ProcessMonitoringFrame(roi *image.RGBA, now time.Time) {
	if roi != nil {
		f.events <- evtMonitoringFrame{roi: roi, now: now}
	}
}
func (f *FishingFSM) TargetCoordinates() (int, int, bool) {
	if !f.coordSet {
		return 0, 0, false
	}
	return f.coordX, f.coordY, true
}
func (f *FishingFSM) Close() {
	if f.closed {
		return
	}
	if f.searchTimer != nil {
		f.searchTimer.Stop()
		f.searchTimer = nil
	}
	if f.cooldownTimer != nil {
		f.cooldownTimer.Stop()
		f.cooldownTimer = nil
	}
	close(f.events)
}

// recoverLog recovers from a panic and logs the error if a logger is available.
func recoverLog(logger *slog.Logger, msg string) {
	if r := recover(); r != nil {
		if logger != nil {
			logger.Error(msg, "error", r)
		}
	}
}

// Ensure contract satisfaction
var _ FishingFSMContract = (*FishingFSM)(nil)
