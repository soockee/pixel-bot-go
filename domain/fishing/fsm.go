package fishing

import (
	"context"
	"image"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

const (
	searchTimeout  = 5 * time.Second        // recast when no bobber appears
	reelClickDelay = 300 * time.Millisecond // gives the cursor time to settle on the bobber
	interactHold   = 40 * time.Millisecond
)

// FishingFSM manages fishing state, its deadline and input actions. All of them
// run on one goroutine, so a transition cancels anything the old state had pending.
type FishingFSM struct {
	state         FishingState // owned by the loop goroutine; cur mirrors it for other goroutines
	cur           atomic.Int32
	coordMu       sync.Mutex // guards coordX/Y/Set writes against TargetCoordinates reads
	coordX        int
	coordY        int
	coordSet      bool
	targetValid   func() bool
	confirmMarker func() bool
	logger        *slog.Logger
	source        *config.Config // copied by the goroutine that starts a run, which owns it
	cfg           config.Config  // loop-owned copy for the current run
	timer         *time.Timer
	gen           uint64 // identifies the live deadline
	deadline      time.Time
	cursorStart   time.Time
	cursorSteps   []CursorStep
	reelDone      func()
	reelTiming    ReelTiming
	reelPressed   bool
	releaseInput  func()
	detectorAt    time.Time
	reelDelay     time.Duration
	failedCasts   int
	sessionEnd    time.Time
	stats         struct{ casts, reels, failedInRow atomic.Int32 } // this run, for the UI
	actions       ActionCallbacks
	events        chan any
	done          <-chan struct{}
	stopped       chan struct{}
	cancel        context.CancelFunc
	listeners     []FishingStateListener
}

// NewFSM creates and starts a FishingFSM in StateHalt. A nil logger discards
// logs and a nil cfg uses the defaults.
func NewFSM(logger *slog.Logger, cfg *config.Config, actions ActionCallbacks) *FishingFSM {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &FishingFSM{
		state: StateHalt, logger: logger, source: cfg, cfg: *cfg, actions: actions,
		events: make(chan any, 64), done: ctx.Done(), stopped: make(chan struct{}), cancel: cancel,
	}
	f.cur.Store(int32(StateHalt))
	go f.loop()
	return f
}

func (f *FishingFSM) loop() {
	defer close(f.stopped)
	defer f.disarm()
	defer f.cancelReel("shutdown")
	defer func() {
		if r := recover(); r != nil {
			f.logger.Error("fsm panic; halting", "error", r, "stack", string(debug.Stack()))
			f.cancel()
			f.transition(StateHalt)
		}
	}()
	for {
		select {
		case <-f.done:
			return
		case ev := <-f.events:
			select {
			case <-f.done:
				return
			default:
			}
			f.handle(ev)
		}
	}
}

func (f *FishingFSM) handle(ev any) {
	switch e := ev.(type) {
	case evtAddListener:
		f.listeners = append(f.listeners, e.l)
	case evtTargetAcquiredAt:
		if f.state == StateSearching {
			f.setCoords(e.x, e.y, true)
			f.targetValid = e.valid
			f.confirmMarker = e.confirm
			f.transition(StateMonitoring)
		}
	case evtTargetInvalidated:
		if (f.state == StateMonitoring || f.state == StateReeling) && !f.hasCurrentTarget() {
			f.resumeSearch()
		}
	case evtTargetLost:
		if f.state == StateMonitoring {
			f.castFailed()
		}
	case evtHalt:
		f.halt()
	case evtFishBite:
		if f.state == StateMonitoring {
			f.detectorAt = e.at
			f.transition(StateReeling)
		}
	case evtFocusAcquired:
		if f.state == StateWaitingFocus {
			f.transition(StateSearching)
		}
	case evtFocusLost:
		if f.state != StateHalt {
			f.transition(StateWaitingFocus)
		}
	case evtAwaitFocus:
		if f.state == StateHalt {
			f.cfg = e.cfg
			f.failedCasts = 0
			f.stats.casts.Store(0)
			f.stats.reels.Store(0)
			f.stats.failedInRow.Store(0)
			if f.actions.Cursor != nil {
				f.actions.Cursor.Reset()
			}
			limit := f.cfg.MaxSessionMinutes
			if f.cfg.Schedule {
				limit = max(limit, f.cfg.PlayMaxMinutes+1)
			}
			f.sessionEnd = time.Now().Add(time.Duration(limit) * time.Minute)
			f.transition(StateWaitingFocus)
		}
	case evtForceCast:
		if f.state == StateSearching || f.state == StateMonitoring || f.state == StateCooldown {
			f.recast()
		}
	case evtDeadline:
		if e.gen == f.gen {
			f.wake()
		}
	}
}

// internal event types sent to the FSM loop
type (
	evtTargetAcquiredAt struct {
		x, y    int
		valid   func() bool
		confirm func() bool
	}
	evtTargetInvalidated struct{}
	evtTargetLost        struct{}
	evtHalt              struct{}
	evtFishBite          struct{ at time.Time }
	evtFocusAcquired     struct{}
	evtFocusLost         struct{}
	evtAwaitFocus        struct{ cfg config.Config }
	evtForceCast         struct{}
	evtAddListener       struct{ l FishingStateListener }
	evtDeadline          struct{ gen uint64 }
)

// arm replaces the pending deadline with one d from now.
func (f *FishingFSM) arm(d time.Duration) {
	f.deadline = time.Now().Add(d)
	f.schedule()
}

// disarm drops the pending deadline. A timer that already fired has an old
// generation, so its event is ignored.
func (f *FishingFSM) disarm() {
	f.deadline = time.Time{}
	f.cursorSteps = nil
	f.stopTimer()
}

func (f *FishingFSM) stopTimer() {
	f.gen++
	if f.timer != nil {
		f.timer.Stop()
	}
}

// schedule uses the same timer for the next cursor step and the state deadline.
func (f *FishingFSM) schedule() {
	f.stopTimer()
	at := f.deadline
	if len(f.cursorSteps) > 0 {
		next := f.cursorStart.Add(f.cursorSteps[0].After)
		if at.IsZero() || next.Before(at) {
			at = next
		}
	}
	if !at.IsZero() {
		gen := f.gen
		f.timer = time.AfterFunc(time.Until(at), func() { f.send(evtDeadline{gen}) })
	}
}

func (f *FishingFSM) wake() {
	now := time.Now()
	due := !f.deadline.IsZero() && !now.Before(f.deadline)
	// Monitoring timeout takes precedence. A reel, in contrast, must reach
	// the exact target before dispatch even if the timer woke late.
	if due && f.state == StateMonitoring {
		f.deadlineReached()
		return
	}
	n := 0
	for n < len(f.cursorSteps) && !now.Before(f.cursorStart.Add(f.cursorSteps[n].After)) {
		n++
	}
	if n > 0 {
		if !f.hasCurrentTarget() {
			f.resumeSearch()
			return
		}
		point := f.cursorSteps[n-1].Point
		f.cursorSteps = f.cursorSteps[n:]
		// Skip elapsed intermediate points instead of sending a burst.
		if !f.actions.Cursor.Move(point) {
			f.halt()
			return
		}
	}
	if due && len(f.cursorSteps) == 0 {
		f.deadlineReached()
		return
	}
	f.schedule()
}

func (f *FishingFSM) deadlineReached() {
	switch f.state {
	case StateSearching, StateMonitoring:
		f.castFailed()
	case StateReeling:
		f.finishReel()
	case StateCooldown:
		f.recast()
	}
}

// castFailed ends a cast that produced no reel, halting once too many fail in a row.
func (f *FishingFSM) castFailed() {
	f.failedCasts++
	f.stats.failedInRow.Store(int32(f.failedCasts))
	if f.failedCasts >= f.cfg.MaxFailedCasts {
		f.logger.Warn("halting: too many failed casts", "limit", f.cfg.MaxFailedCasts)
		f.halt()
		return
	}
	f.recast()
}

func (f *FishingFSM) recast() {
	if time.Now().After(f.sessionEnd) {
		f.logger.Warn("halting: session limit reached", "minutes", f.cfg.MaxSessionMinutes)
		f.halt()
		return
	}
	f.transition(StateCasting)
}

func (f *FishingFSM) halt() {
	f.setCoords(0, 0, false)
	f.transition(StateHalt)
}

func (f *FishingFSM) setCoords(x, y int, set bool) {
	f.coordMu.Lock()
	defer f.coordMu.Unlock()
	f.coordX, f.coordY, f.coordSet = x, y, set
}

func (f *FishingFSM) transition(next FishingState) {
	prev := f.state
	if prev == next {
		return
	}
	f.disarm()
	if prev == StateReeling && next != StateCooldown {
		f.cancelReel(next.String())
	}
	f.state = next
	f.cur.Store(int32(next))
	f.logger.Info("fishing state transition", "from", prev.String(), "to", next.String())
	for _, l := range f.listeners {
		l(prev, next)
	}
	switch next {
	case StateSearching:
		f.arm(searchTimeout)
	case StateCasting:
		if f.pressCast() {
			f.transition(StateSearching)
		} else {
			f.halt()
		}
	case StateMonitoring:
		f.startMonitoring()
	case StateReeling:
		f.startReel()
	case StateCooldown:
		cooldown := f.reelTiming.Cooldown
		f.reelTiming = ReelTiming{}
		if cooldown == 0 {
			cooldown = time.Duration(f.cfg.CooldownSeconds) * time.Second
		}
		f.arm(cooldown)
	}
}

// pressCast reports false when the configured key is unusable, so the wrong
// key is never pressed.
func (f *FishingFSM) pressCast() bool {
	vk, ok := f.actions.ParseVK(f.cfg.CastKey)
	if !ok {
		f.logger.Error("halting: unknown cast key", "key", f.cfg.CastKey)
		return false
	}
	var dispatched func()
	if f.actions.ObserveCast != nil {
		dispatched = f.actions.ObserveCast()
	}
	if f.hardwareKeyboard() {
		if !f.tapKeyboard(vk) {
			return false
		}
	} else {
		f.actions.PressKey(vk)
	}
	if dispatched != nil {
		dispatched()
	}
	f.stats.casts.Add(1)
	f.logger.Info("cast action executed", "key", f.cfg.CastKey)
	return true
}

func (f *FishingFSM) startMonitoring() {
	if !f.hasCurrentTarget() {
		f.resumeSearch()
		return
	}
	if f.cfg.ReelMethod != config.ReelInteractKey && !f.beginCursor(true) {
		return
	}
	f.logger.Info("found blobber", "x", f.coordX, "y", f.coordY)
	f.arm(time.Duration(f.cfg.MaxCastDurationSeconds) * time.Second)
}

func (f *FishingFSM) startReel() {
	f.reelDone = nil
	if !f.coordSet {
		f.logger.Info("reel action skipped - no target coords")
		f.transition(StateCooldown)
		return
	}
	if !f.hasCurrentTarget() {
		f.resumeSearch()
		return
	}
	if f.actions.ObserveReel != nil {
		f.reelDone = f.actions.ObserveReel()
	}
	f.reelDelay = reelClickDelay
	if f.actions.PlanReel != nil {
		f.reelTiming = f.actions.PlanReel(f.cfg.BiteDetector)
		if f.reelTiming.Cooldown == 0 {
			f.reelTiming.Cooldown = time.Duration(f.cfg.CooldownSeconds) * time.Second
		}
		f.reelPressed = false
	}
	if f.cfg.ReelMethod == config.ReelInteractKey {
		f.reelTiming.Hold = interactHold
		if f.hardwareKeyboard() {
			f.reelTiming.Hold = 0 // the ESP32 owns the press and release
		}
	} else if !f.beginCursor(false) {
		return
	}
	if f.reelTiming.Hold > 0 {
		if elapsed := f.reelTiming.AfterDetection; elapsed > 0 {
			f.reelDelay = max(0, elapsed-time.Since(f.detectorAt))
		}
	}
	if n := len(f.cursorSteps); n > 0 {
		f.reelDelay = max(f.reelDelay, time.Until(f.cursorStart.Add(f.cursorSteps[n-1].After)))
	}
	if f.reelTiming.Hold > 0 {
		f.observeReelTiming("reel_planned", "")
	}
	f.arm(f.reelDelay)
}

func (f *FishingFSM) beginCursor(settle bool) bool {
	if f.actions.Cursor == nil {
		f.actions.MoveCursor(f.coordX, f.coordY)
		return true
	}
	target := image.Pt(f.coordX, f.coordY)
	steps, ok := f.actions.Cursor.Plan(target, settle)
	if !ok || len(steps) == 0 || steps[len(steps)-1].Point != target {
		f.logger.Warn("halting: cursor path unavailable")
		f.halt()
		return false
	}
	f.cursorStart, f.cursorSteps = time.Now(), steps
	return true
}

func (f *FishingFSM) finishReel() {
	if !f.hasCurrentTarget() {
		f.resumeSearch()
		return
	}
	if f.actions.CanSimulateReel != nil && !f.actions.CanSimulateReel() {
		f.transition(StateWaitingFocus)
		return
	}
	if !f.reelPressed {
		needsRelease := false
		if f.cfg.ReelMethod == config.ReelInteractKey {
			vk, ok := f.interactVK(f.cfg)
			if !ok {
				f.logger.Warn("halting: Interact key unavailable")
				f.halt()
				return
			}
			if f.confirmMarker == nil || !f.confirmMarker() || !f.hasCurrentTarget() {
				f.logger.Info("reel action skipped: bobber marker no longer confirmed")
				f.resumeSearch()
				return
			}
			if f.hardwareKeyboard() {
				if !f.tapKeyboard(vk) {
					f.halt()
					return
				}
			} else {
				needsRelease = true
				if f.actions.HoldInteract != nil {
					f.releaseInput = f.actions.HoldInteract(vk)
				}
			}
		} else if f.reelTiming.Hold > 0 && f.actions.HoldRight != nil {
			needsRelease = true
			f.releaseInput = f.actions.HoldRight()
		} else {
			f.actions.ClickRight()
		}
		if needsRelease && f.releaseInput == nil {
			f.logger.Warn("halting: timed reel press was blocked")
			f.halt()
			return
		}
		if f.reelDone != nil {
			f.reelDone()
		}
		if f.reelTiming.Hold > 0 {
			f.reelPressed = true
			f.arm(f.reelTiming.Hold)
			return
		}
	} else {
		f.releaseReel()
		f.observeReelTiming("reel_released", "")
		f.reelPressed = false
	}
	f.failedCasts = 0
	f.stats.reels.Add(1)
	f.stats.failedInRow.Store(0)
	f.logger.Info("reel action executed", "x", f.coordX, "y", f.coordY, "method", f.cfg.ReelMethod)
	f.transition(StateCooldown)
}

func (f *FishingFSM) hardwareKeyboard() bool {
	return f.cfg.ESP32Port != "" && !f.cfg.DryRun
}

func (f *FishingFSM) tapKeyboard(vk byte) bool {
	if f.actions.TapKeyboard == nil {
		f.logger.Warn("halting: ESP32 keyboard unavailable")
		return false
	}
	if err := f.actions.TapKeyboard(vk); err != nil {
		f.logger.Error("halting: ESP32 keyboard action failed", "error", err)
		return false
	}
	return true
}

func (f *FishingFSM) interactVK(cfg config.Config) (byte, bool) {
	vk, ok := f.actions.ParseVK(cfg.InteractKey)
	cast, castOK := f.actions.ParseVK(cfg.CastKey)
	stop, stopOK := f.actions.ParseVK(cfg.StopKey)
	return vk, ok && !(castOK && cast == vk) && !(stopOK && stop == vk)
}

func (f *FishingFSM) hasCurrentTarget() bool {
	return f.coordSet && (f.targetValid == nil || f.targetValid())
}

func (f *FishingFSM) resumeSearch() {
	f.setCoords(0, 0, false)
	f.transition(StateSearching)
}

func (f *FishingFSM) observeReelTiming(kind, reason string) {
	if f.actions.ObserveReelTiming != nil {
		f.actions.ObserveReelTiming(ReelTimingEvent{
			Kind: kind, Reason: reason, Detector: f.cfg.BiteDetector,
			DetectorAt: f.detectorAt, Timing: f.reelTiming, Delay: f.reelDelay,
		})
	}
}

func (f *FishingFSM) cancelReel(reason string) {
	f.releaseReel()
	if f.reelTiming.Hold > 0 {
		f.observeReelTiming("reel_canceled", reason)
	}
	f.reelTiming = ReelTiming{}
	f.reelPressed = false
}

func (f *FishingFSM) releaseReel() {
	if release := f.releaseInput; release != nil {
		f.releaseInput = nil
		release()
	}
}

// send queues an event, or drops it once the FSM has shut down.
func (f *FishingFSM) send(ev any) {
	select {
	case f.events <- ev:
	case <-f.done:
	}
}

// Public API methods
func (f *FishingFSM) AddListener(l FishingStateListener) { f.send(evtAddListener{l: l}) }
func (f *FishingFSM) Current() FishingState              { return FishingState(f.cur.Load()) }
func (f *FishingFSM) EventTargetAcquiredAt(x, y int)     { f.send(evtTargetAcquiredAt{x: x, y: y}) }
func (f *FishingFSM) EventTargetLost()                   { f.send(evtTargetLost{}) }
func (f *FishingFSM) EventHalt()                         { f.send(evtHalt{}) }
func (f *FishingFSM) EventFishBite()                     { f.send(evtFishBite{at: time.Now()}) }
func (f *FishingFSM) EventFocusAcquired()                { f.send(evtFocusAcquired{}) }
func (f *FishingFSM) EventFocusLost()                    { f.send(evtFocusLost{}) }
func (f *FishingFSM) ForceCast()                         { f.send(evtForceCast{}) }

// EventAwaitFocus starts a run with a copy of the current config. Call it from
// the goroutine that owns the config.
func (f *FishingFSM) EventAwaitFocus() { f.send(evtAwaitFocus{cfg: *f.source}) }

// TargetCoordinates returns the locked target, if any.
func (f *FishingFSM) TargetCoordinates() (int, int, bool) {
	f.coordMu.Lock()
	defer f.coordMu.Unlock()
	if !f.coordSet {
		return 0, 0, false
	}
	return f.coordX, f.coordY, true
}

// Stats counts the current run's actions; a new run starts them at zero.
type Stats struct {
	Casts       int // casts made
	Reels       int // reels made, each one a bite the bot answered
	FailedInRow int // casts in a row that ended without a reel
}

// Stats returns the current run's counts. Safe from any goroutine.
func (f *FishingFSM) Stats() Stats {
	return Stats{Casts: int(f.stats.casts.Load()), Reels: int(f.stats.reels.Load()), FailedInRow: int(f.stats.failedInRow.Load())}
}

// Close stops the FSM and waits for its loop. Call it outside that loop.
func (f *FishingFSM) Close() {
	f.cancel()
	<-f.stopped
}
