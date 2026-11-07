package fishing

import (
	"context"
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
)

// FishingFSM manages fishing state, its deadline and input actions. All of them
// run on one goroutine, so a transition cancels anything the old state had pending.
type FishingFSM struct {
	state       FishingState // owned by the loop goroutine; cur mirrors it for other goroutines
	cur         atomic.Int32
	coordMu     sync.Mutex // guards coordX/Y/Set writes against TargetCoordinates reads
	coordX      int
	coordY      int
	coordSet    bool
	logger      *slog.Logger
	source      *config.Config // copied by the goroutine that starts a run, which owns it
	cfg         config.Config  // loop-owned copy for the current run
	timer       *time.Timer
	gen         uint64 // identifies the live deadline
	reelDone    func()
	failedCasts int
	sessionEnd  time.Time
	actions     ActionCallbacks
	events      chan any
	done        <-chan struct{}
	cancel      context.CancelFunc
	listeners   []FishingStateListener
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
		events: make(chan any, 64), done: ctx.Done(), cancel: cancel,
	}
	f.cur.Store(int32(StateHalt))
	go f.loop()
	return f
}

func (f *FishingFSM) loop() {
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
			f.disarm()
			return
		case ev := <-f.events:
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
			f.transition(StateMonitoring)
		}
	case evtTargetLost:
		if f.state == StateMonitoring {
			f.castFailed()
		}
	case evtHalt:
		f.halt()
	case evtFishBite:
		if f.state == StateMonitoring {
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
			f.sessionEnd = time.Now().Add(time.Duration(f.cfg.MaxSessionMinutes) * time.Minute)
			f.transition(StateWaitingFocus)
		}
	case evtForceCast:
		if f.state == StateSearching || f.state == StateMonitoring || f.state == StateCooldown {
			f.recast()
		}
	case evtDeadline:
		if e.gen == f.gen {
			f.deadlineReached()
		}
	}
}

// internal event types sent to the FSM loop
type (
	evtTargetAcquiredAt struct{ x, y int }
	evtTargetLost       struct{}
	evtHalt             struct{}
	evtFishBite         struct{}
	evtFocusAcquired    struct{}
	evtFocusLost        struct{}
	evtAwaitFocus       struct{ cfg config.Config }
	evtForceCast        struct{}
	evtAddListener      struct{ l FishingStateListener }
	evtDeadline         struct{ gen uint64 }
)

// arm replaces the pending deadline with one d from now.
func (f *FishingFSM) arm(d time.Duration) {
	f.disarm()
	gen := f.gen
	f.timer = time.AfterFunc(d, func() { f.send(evtDeadline{gen}) })
}

// disarm drops the pending deadline. A timer that already fired has an old
// generation, so its event is ignored.
func (f *FishingFSM) disarm() {
	f.gen++
	if f.timer != nil {
		f.timer.Stop()
	}
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
		f.arm(time.Duration(f.cfg.CooldownSeconds) * time.Second)
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
	f.actions.PressKey(vk)
	if dispatched != nil {
		dispatched()
	}
	f.logger.Info("cast action executed", "key", f.cfg.CastKey)
	return true
}

func (f *FishingFSM) startMonitoring() {
	if f.coordSet {
		f.actions.MoveCursor(f.coordX, f.coordY)
		f.logger.Info("found blobber", "x", f.coordX, "y", f.coordY)
	}
	f.arm(time.Duration(f.cfg.MaxCastDurationSeconds) * time.Second)
}

func (f *FishingFSM) startReel() {
	f.reelDone = nil
	if !f.coordSet {
		f.logger.Info("reel action skipped - no target coords")
		f.transition(StateCooldown)
		return
	}
	if f.actions.ObserveReel != nil {
		f.reelDone = f.actions.ObserveReel()
	}
	f.actions.MoveCursor(f.coordX, f.coordY)
	f.arm(reelClickDelay)
}

func (f *FishingFSM) finishReel() {
	f.actions.ClickRight()
	if f.reelDone != nil {
		f.reelDone()
	}
	f.failedCasts = 0
	f.logger.Info("reel action executed", "x", f.coordX, "y", f.coordY)
	f.transition(StateCooldown)
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
func (f *FishingFSM) EventFishBite()                     { f.send(evtFishBite{}) }
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

// Close stops the FSM. Later events are dropped, and calling it again is safe.
func (f *FishingFSM) Close() { f.cancel() }
