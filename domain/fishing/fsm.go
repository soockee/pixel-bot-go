package fishing

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

const (
	searchTimeout    = 5 * time.Second        // recast when no bobber appears
	reelClickDelay   = 300 * time.Millisecond // gives the cursor time to settle on the bobber
	interactHold     = 40 * time.Millisecond
	repositionSettle = 750 * time.Millisecond
)

// FishingFSM manages fishing state, its deadline and input actions. All of them
// run on one goroutine, so a transition cancels anything the old state had pending.
type FishingFSM struct {
	state               FishingState // owned by the loop goroutine; cur mirrors it for other goroutines
	cur                 atomic.Int32
	coordMu             sync.Mutex // guards coordX/Y/Set writes against TargetCoordinates reads
	coordX              int
	coordY              int
	coordSet            bool
	targetValid         func() bool
	confirmMarker       func() bool
	targetBox           image.Rectangle
	missAim             *image.Point
	offTargetReel       bool
	logger              *slog.Logger
	source              *config.Config // copied by the goroutine that starts a run, which owns it
	cfg                 config.Config  // loop-owned copy for the current run
	timer               *time.Timer
	gen                 uint64 // identifies the live deadline
	deadline            time.Time
	cursorStart         time.Time
	cursorSteps         []CursorStep
	reelDone            func()
	reelTiming          ReelTiming
	reelPressed         bool
	releaseInput        func()
	detectorAt          time.Time
	reelDelay           time.Duration
	idleTried           bool
	outcomesShown       bool          // Bobber Marker showed its outcome blocks this run
	awaitOutcome        bool          // the cooldown about to start follows a reel that should bring an outcome
	castPause           time.Duration // the pause before the next cast while the cooldown waits for the outcome
	nextReposition      int32
	checkingReposition  bool
	repositionAttempt   int
	repositionLimit     int
	repositionDirection int
	repositionDX        int
	repositionLogout    atomic.Bool
	failedCasts         int
	sessionEnd          time.Time
	stats               struct{ casts, reels, reelAttempts, reelInputs, failedInRow, caught, gotAway, notHooked atomic.Int32 } // this run, for the UI
	actions             ActionCallbacks
	events              chan any
	done                <-chan struct{}
	stopped             chan struct{}
	cancel              context.CancelFunc
	listeners           []FishingStateListener
	haltReason          atomic.Pointer[string] // why the run halted on its own; nil when the person stopped it
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
			reason := "an internal error; see pixel_bot_logs.json"
			f.haltReason.Store(&reason)
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
			f.targetBox = e.box
			f.missAim = e.missAim
			if f.checkingReposition && f.hasCurrentTarget() {
				f.checkingReposition = false
				f.failedCasts = 0
				f.stats.failedInRow.Store(0)
				f.planReposition()
				f.logger.Info("reposition verified: bobber found")
			}
			f.transition(StateMonitoring)
		}
	case evtTargetInvalidated:
		if (f.state == StateMonitoring || f.state == StateReeling) && !f.hasCurrentTarget() {
			f.resumeSearch()
		}
	case evtPassingPlayer:
		if f.state == StateMonitoring && f.hasCurrentTarget() && e.confirm() {
			f.activity(f.cfg.TargetPlayerKey, "target passing player")
		}
	case evtTargetLost:
		if f.state == StateMonitoring {
			f.castFailed()
		}
	case evtHalt:
		if e.reason != "" {
			f.why(e.reason)
		}
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
		if f.checkingReposition && f.state != StateHalt {
			f.why("game focus was lost while checking the new fishing position")
			f.halt()
		} else if f.state != StateHalt {
			f.transition(StateWaitingFocus)
		}
	case evtAwaitFocus:
		if f.state == StateHalt {
			f.cfg = e.cfg
			f.haltReason.Store(nil)
			f.failedCasts = 0
			f.stats.casts.Store(0)
			f.stats.reels.Store(0)
			f.stats.reelAttempts.Store(0)
			f.stats.reelInputs.Store(0)
			f.stats.caught.Store(0)
			f.stats.gotAway.Store(0)
			f.stats.notHooked.Store(0)
			f.stats.failedInRow.Store(0)
			f.outcomesShown = false
			f.checkingReposition = false
			f.repositionLogout.Store(false)
			f.planReposition()
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
		if !f.checkingReposition && (f.state == StateSearching || f.state == StateMonitoring || f.state == StateCooldown) {
			f.recast()
		}
	case evtOutcome:
		f.outcomesShown = true
		f.countOutcome(e.o)
		if e.o != OutcomeNone && f.state == StateCooldown && f.castPause > 0 {
			f.arm(f.castPause)
			f.castPause = 0
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
		missAim *image.Point
		box     image.Rectangle
	}
	evtTargetInvalidated struct{}
	evtTargetLost        struct{}
	evtPassingPlayer     struct{ confirm func() bool }
	evtHalt              struct{ reason string }
	evtFishBite          struct{ at time.Time }
	evtFocusAcquired     struct{}
	evtFocusLost         struct{}
	evtAwaitFocus        struct{ cfg config.Config }
	evtForceCast         struct{}
	evtAddListener       struct{ l FishingStateListener }
	evtDeadline          struct{ gen uint64 }
	evtOutcome           struct{ o Outcome }
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
			f.why("the pointer was moved away, or the game was not in front")
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
		f.castPause = 0 // the outcome did not come in time
		if !f.idleTried && time.Now().Before(f.sessionEnd) {
			if f.nextReposition != 0 && f.stats.reels.Load() >= f.nextReposition {
				f.startReposition()
				return
			}
			if f.cfg.IdleMoves && !f.cfg.Background && rand.IntN(12) == 0 { // a background run leaves the pointer to the person
				f.idleTried = true
				if f.actions.CanAct != nil && !f.actions.CanAct() {
					f.transition(StateWaitingFocus)
					return
				}
				f.idleMove()
				f.arm(time.Duration(300+rand.IntN(601)) * time.Millisecond)
				return
			}
		}
		f.recast()
	}
}

// castFailed ends a cast that produced no reel, halting once too many fail in a row.
func (f *FishingFSM) countFailedCast() bool {
	f.failedCasts++
	f.stats.failedInRow.Store(int32(f.failedCasts))
	return f.failedCasts >= f.cfg.MaxFailedCasts
}

func (f *FishingFSM) castFailed() {
	if f.checkingReposition {
		f.countFailedCast()
		if time.Now().After(f.sessionEnd) {
			f.recast()
			return
		}
		if f.repositionAttempt < f.repositionLimit {
			f.transition(StateCooldown)
			f.startReposition()
			return
		}
		f.why(fmt.Sprintf("no bobber found after %d reposition attempts", f.repositionAttempt))
		f.repositionLogout.Store(true)
		f.halt()
		return
	}
	if f.countFailedCast() {
		f.why(fmt.Sprintf("%d casts in a row got no bite", f.cfg.MaxFailedCasts))
		f.halt()
		return
	}
	f.recast()
}

func (f *FishingFSM) planReposition() {
	f.nextReposition = 0
	if f.cfg.Reposition {
		f.nextReposition = f.stats.reels.Load() + int32(45+rand.IntN(11))
	}
}

func (f *FishingFSM) startReposition() {
	f.idleTried = true
	if !f.checkingReposition {
		f.repositionAttempt = 0
		f.repositionLimit = 4 + rand.IntN(3) // initial move plus three to five retries
		f.repositionDirection = rand.IntN(12)
		f.repositionDX = 12
		if roll := f.actions.RepositionRoll; roll != nil {
			f.repositionDX = 8 + roll(9)
			if roll(2) == 0 {
				f.repositionDX = -f.repositionDX
			}
		}
	}
	if !f.moveForReposition() {
		if f.state != StateHalt {
			f.why("reposition input was blocked. Check the movement binding and game focus")
			f.halt()
		}
		return
	}
	f.repositionAttempt++
	f.checkingReposition = true
	settle := repositionSettle
	if roll := f.actions.RepositionRoll; roll != nil {
		settle = time.Duration(550+roll(251)+roll(251)) * time.Millisecond
	}
	f.arm(settle)
}

func (f *FishingFSM) moveForReposition() bool {
	// Reverse each pair of steps before trying the other axis.
	direction := f.repositionDirection % 4
	key := string("WSAD"[direction^(f.repositionAttempt%4)])
	cameraAttempt := f.repositionAttempt - 2
	if f.repositionDirection < 4 {
		cameraAttempt = f.repositionAttempt
	}
	if drag := f.cameraDrag(); drag != nil && cameraAttempt >= 0 && cameraAttempt < 2 {
		dx := f.repositionDX
		if dx == 0 {
			dx = 12
		}
		if cameraAttempt == 1 {
			dx = -dx
		}
		if attempted, moved := drag(dx); attempted {
			if moved {
				f.logger.Info("reposition camera drag", "dx", dx)
			}
			return moved
		}
	}
	return f.activity(key, "reposition action executed")
}

// TakeRepositionLogout reports a failed reposition sequence once, after capture stops.
func (f *FishingFSM) TakeRepositionLogout() bool {
	return f.repositionLogout.Swap(false)
}

func (f *FishingFSM) recast() {
	if time.Now().After(f.sessionEnd) {
		f.why(fmt.Sprintf("the %d minute session limit was reached", f.cfg.MaxSessionMinutes))
		f.halt()
		return
	}
	f.transition(StateCasting)
}

func (f *FishingFSM) halt() {
	f.setCoords(0, 0, false)
	f.transition(StateHalt)
}

// why records the reason for a halt that the person did not ask for, before
// halt notifies the listeners that show it.
func (f *FishingFSM) why(reason string) {
	f.logger.Warn("halting: " + reason)
	f.haltReason.Store(&reason)
}

// HaltReason says why the last run halted on its own, or "" when the person
// stopped it. Safe from any goroutine.
func (f *FishingFSM) HaltReason() string {
	if reason := f.haltReason.Load(); reason != nil {
		return *reason
	}
	return ""
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
		f.idleTried = false
		cooldown := f.reelTiming.Cooldown
		f.reelTiming = ReelTiming{}
		if cooldown == 0 {
			cooldown = f.pause()
		}
		f.castPause = 0
		if f.awaitOutcome {
			f.awaitOutcome = false
			f.castPause, cooldown = cooldown, outcomeWait
		}
		f.arm(cooldown)
	}
}

// pressCast reports false when the configured key is unusable, so the wrong
// key is never pressed.
func (f *FishingFSM) pressCast() bool {
	vk, ok := f.actions.ParseVK(f.cfg.CastKey)
	if !ok {
		f.why(fmt.Sprintf("%q is not a Cast key Go Fish can press", f.cfg.CastKey))
		return false
	}
	var dispatched func()
	if f.actions.ObserveCast != nil {
		dispatched = f.actions.ObserveCast()
	}
	if f.hardwareKeyboard() {
		if !f.tapKeyboard(vk, 0) {
			return false
		}
	} else if !f.actions.PressKey(vk) {
		f.why("cast input was blocked")
		return false
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
	if f.cfg.ReelMethod != config.ReelInteractKey {
		aim, move := image.Pt(f.coordX, f.coordY), true
		if f.actions.Approach != nil {
			aim, move = f.actions.Approach(aim, f.targetBox)
		}
		if move && !f.beginCursor(aim, true) {
			return
		}
	}
	f.logger.Info("found blobber", "x", f.coordX, "y", f.coordY)
	f.arm(time.Duration(f.cfg.MaxCastDurationSeconds) * time.Second)
}

// reelMissOdds is the n of the reel roll: outcomes 0 and 1 of [0, n) miss on
// purpose, so 2/n of bites, 5%.
const reelMissOdds = 40

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
	f.offTargetReel = false
	aim := image.Pt(f.coordX, f.coordY)
	if f.actions.ReelRoll != nil {
		// One bite in reelMissOdds is not reeled, one is clicked beside the
		// bobber: 5% deliberate misses, as a person makes.
		switch f.actions.ReelRoll(reelMissOdds) {
		case 0:
			f.finishMiss("forgot to reel")
			return
		case 1:
			if f.cfg.ReelMethod == config.ReelInteractKey || f.missAim == nil || f.actions.HoldRight == nil {
				f.finishMiss("no off-target reel available")
				return
			}
			f.offTargetReel = true
			aim = *f.missAim
		}
	}
	if f.actions.ObserveReel != nil {
		f.reelDone = f.actions.ObserveReel()
	}
	f.reelDelay = reelClickDelay
	if f.actions.PlanReel != nil {
		f.reelTiming = f.actions.PlanReel(f.cfg.BiteDetector)
		if f.reelTiming.Cooldown == 0 {
			f.reelTiming.Cooldown = f.pause()
		}
		f.reelPressed = false
	}
	if f.cfg.ReelMethod == config.ReelInteractKey {
		if f.reelTiming.Hold == 0 {
			f.reelTiming.Hold = interactHold
		}
	} else {
		if f.offTargetReel && f.reelTiming.Hold == 0 {
			f.reelTiming.Hold = interactHold
		}
		if !f.beginCursor(aim, false) {
			return
		}
	}
	if elapsed := f.reelTiming.AfterDetection; elapsed > 0 {
		f.reelDelay = max(0, elapsed-time.Since(f.detectorAt))
	}
	if f.hardwareKeyboard() && f.cfg.ReelMethod != config.ReelInteractKey {
		f.reelTiming.Hold = 0 // the ESP32 right click owns its press and release
	}
	if n := len(f.cursorSteps); n > 0 {
		f.reelDelay = max(f.reelDelay, time.Until(f.cursorStart.Add(f.cursorSteps[n-1].After)))
	}
	if f.reelTiming.Hold > 0 {
		f.observeReelTiming("reel_planned", "")
	}
	f.arm(f.reelDelay)
}

func (f *FishingFSM) beginCursor(target image.Point, settle bool) bool {
	if f.actions.Cursor == nil {
		f.actions.MoveCursor(target.X, target.Y)
		return true
	}
	steps, ok := f.actions.Cursor.Plan(target, settle)
	if !ok || len(steps) == 0 || steps[len(steps)-1].Point != target {
		f.why("could not aim at the bobber. Keep the pointer inside the focused game window")
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
				f.why("the Interact key is not set, or it matches the Cast key")
				f.halt()
				return
			}
			if f.confirmMarker == nil || !f.confirmMarker() || !f.hasCurrentTarget() {
				f.logger.Info("reel action skipped: bobber marker no longer confirmed")
				f.resumeSearch()
				return
			}
			f.stats.reelAttempts.Add(1)
			if f.hardwareKeyboard() {
				if !f.tapKeyboard(vk, f.reelTiming.Hold) {
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
			f.stats.reelAttempts.Add(1)
			needsRelease = true
			f.releaseInput = f.actions.HoldRight()
		} else {
			f.stats.reelAttempts.Add(1)
			if !f.actions.ClickRight() {
				f.why("reel click was blocked")
				f.halt()
				return
			}
		}
		if needsRelease && f.releaseInput == nil {
			f.why("the reel press was blocked; the game was not in front")
			f.halt()
			return
		}
		if f.reelDone != nil {
			f.reelDone()
		}
		if f.reelTiming.Hold > 0 && !f.hardwareKeyboard() {
			f.reelPressed = true
			f.arm(f.reelTiming.Hold)
			return
		}
	} else {
		f.releaseReel()
		f.observeReelTiming("reel_released", "")
		f.reelPressed = false
	}
	f.stats.reelInputs.Add(1)
	if f.offTargetReel {
		f.finishMiss("clicked beside the bobber")
		return
	}
	f.failedCasts = 0
	f.stats.reels.Add(1)
	f.stats.failedInRow.Store(0)
	f.logger.Info("reel action executed", "x", f.coordX, "y", f.coordY, "method", f.cfg.ReelMethod)
	// A cast before the loot opens loses the catch's outcome, so wait for it.
	f.awaitOutcome = f.outcomesShown
	f.transition(StateCooldown)
}

func (f *FishingFSM) finishMiss(reason string) {
	f.logger.Info("reel missed", "reason", reason)
	if f.countFailedCast() {
		f.why(fmt.Sprintf("%d casts in a row ended without a catch", f.cfg.MaxFailedCasts))
		f.halt()
		return
	}
	f.transition(StateCooldown)
	f.idleTried = true
}

func (f *FishingFSM) hardwareKeyboard() bool {
	return f.cfg.ESP32Port != "" && !f.cfg.DryRun
}

func (f *FishingFSM) tapKeyboard(vk byte, hold time.Duration) bool {
	if f.actions.TapKeyboard == nil {
		f.why("the ESP32 keyboard is not available")
		return false
	}
	if err := f.actions.TapKeyboard(vk, hold); err != nil {
		f.why("the ESP32 keyboard failed: " + err.Error())
		return false
	}
	return true
}

// pause is the wait after a catch before the next cast: the configured
// seconds, or by default a quick random 0.4 to 1.2 seconds, as a player
// throws out again right after reeling in. The next search ignores the last
// bobber's marker (markerIsStale) while that bobber fades.
func (f *FishingFSM) pause() time.Duration {
	if f.cfg.CooldownSeconds > 0 {
		return time.Duration(f.cfg.CooldownSeconds) * time.Second
	}
	return quickPauseMin + rand.N(quickPauseMax-quickPauseMin)
}

const quickPauseMin, quickPauseMax = 400 * time.Millisecond, 1200 * time.Millisecond

// outcomeWait is how long a cooldown waits for Bobber Marker's outcome after a
// reel before it casts anyway. The loot opens up to about a second after it.
const outcomeWait = 3 * time.Second

func (f *FishingFSM) interactVK(cfg config.Config) (byte, bool) {
	vk, ok := f.actions.ParseVK(cfg.InteractKey)
	cast, castOK := f.actions.ParseVK(cfg.CastKey)
	return vk, ok && !(castOK && cast == vk)
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

// EventHaltBecause halts and says why, for the UI to show.
func (f *FishingFSM) EventHaltBecause(reason string) { f.send(evtHalt{reason: reason}) }
func (f *FishingFSM) EventFishBite()                 { f.send(evtFishBite{at: time.Now()}) }
func (f *FishingFSM) EventFocusAcquired()            { f.send(evtFocusAcquired{}) }
func (f *FishingFSM) EventFocusLost()                { f.send(evtFocusLost{}) }
func (f *FishingFSM) ForceCast()                     { f.send(evtForceCast{}) }

// EventOutcome reports a reading of Bobber Marker's outcome blocks: a new
// outcome, or OutcomeNone when they still show an earlier one.
func (f *FishingFSM) EventOutcome(o Outcome) { f.send(evtOutcome{o: o}) }

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
	Casts        int // casts made
	Reels        int // acknowledged on-target reel inputs
	ReelAttempts int // dispatch attempts, including blocked and off-target input
	ReelInputs   int // acknowledged reel inputs, including off-target input
	FailedInRow  int // casts in a row that ended without a reel
	// How casts ended, as Bobber Marker reports them (OutcomeSignalArea).
	Caught, GotAway, NotHooked int
}

// Stats returns the current run's counts. Safe from any goroutine.
func (f *FishingFSM) Stats() Stats {
	return Stats{
		Casts: int(f.stats.casts.Load()), Reels: int(f.stats.reels.Load()),
		ReelAttempts: int(f.stats.reelAttempts.Load()), ReelInputs: int(f.stats.reelInputs.Load()),
		FailedInRow: int(f.stats.failedInRow.Load()),
		Caught:      int(f.stats.caught.Load()), GotAway: int(f.stats.gotAway.Load()), NotHooked: int(f.stats.notHooked.Load()),
	}
}

// Close stops the FSM and waits for its loop. Call it outside that loop.
func (f *FishingFSM) Close() {
	f.cancel()
	<-f.stopped
}
