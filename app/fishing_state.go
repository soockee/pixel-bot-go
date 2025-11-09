package app

import (
	"image"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// FishingState represents the high-level finite states of the fishing cycle.
type FishingState int

const (
	StateSearching    FishingState = iota // searching for the target object
	StateMonitoring                       // monitoring target for bite movement
	StateReeling                          // performing reel action (mouse press)
	StateCooldown                         // waiting for loot/timeout before next cast
	StateCasting                          // casting the fishing rod (key press)
	StateHalt                             // currently waiting for actionable state
	StateWaitingFocus                     // waiting for focus on game window
)

func (s FishingState) String() string {
	switch s {
	case StateHalt:
		return "halt"
	case StateSearching:
		return "searching"
	case StateMonitoring:
		return "monitoring"
	case StateReeling:
		return "reeling"
	case StateCooldown:
		return "cooldown"
	case StateCasting:
		return "casting"
	case StateWaitingFocus:
		return "focus"
	default:
		return "unknown"
	}
}

// FishingStateListener is invoked on every successful state transition.
type FishingStateListener func(prev, next FishingState)

// internal events
type fsmEvent interface{}
type (
	evtTick             struct{ now time.Time }
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

// FishingStateMachine runs as a single goroutine (actor). All state mutations
// happen on that goroutine, removing the need for locks and eliminating
// re-entrancy issues.
type FishingStateMachine struct {
	state            FishingState
	logger           *slog.Logger
	cfg              *config.Config
	listeners        []FishingStateListener
	events           chan fsmEvent
	cooldownDuration time.Duration
	cooldownUntil    time.Time
	searchStarted    time.Time
	coordX, coordY   int
	coordSet         bool
	biteDetector     *BiteDetector
	closed           bool
}

// NewFishingStateMachine creates and starts the event loop.
func NewFishingStateMachine(logger *slog.Logger, cfg *config.Config) *FishingStateMachine {
	cooldown := time.Second
	if cfg != nil && cfg.CooldownSeconds > 0 {
		cooldown = time.Duration(cfg.CooldownSeconds) * time.Second
	}
	m := &FishingStateMachine{
		state:            StateHalt,
		logger:           logger,
		cfg:              cfg,
		cooldownDuration: cooldown,
		searchStarted:    time.Now(),
		events:           make(chan fsmEvent, 64),
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("fsm panic", "error", r, "stack", string(debug.Stack()))
				}
			}
		}()
		m.loop()
	}()
	return m
}

// loop processes events sequentially.
func (m *FishingStateMachine) loop() {
	for ev := range m.events {
		switch e := ev.(type) {
		case evtAddListener:
			m.listeners = append(m.listeners, e.l)
		case evtTick:
			m.handleTick(e.now)
		case evtTargetAcquired:
			if m.state == StateSearching {
				m.transition(StateMonitoring)
			}
		case evtTargetAcquiredAt:
			m.coordX, m.coordY, m.coordSet = e.x, e.y, true
			if m.state == StateSearching {
				m.transition(StateMonitoring)
			}
		case evtTargetLost:
			if m.state == StateMonitoring {
				m.transition(StateCasting)
			}
		case evtHalt:
			m.cooldownUntil = time.Time{}
			m.coordSet = false
			if m.biteDetector != nil {
				m.biteDetector.Reset()
			}
			m.transition(StateHalt)
		case evtFishBite:
			if m.state == StateMonitoring {
				m.transition(StateReeling)
			}
		case evtMonitoringFrame:
			if m.state == StateMonitoring && m.biteDetector != nil && e.roi != nil {
				if m.biteDetector.FeedFrame(e.roi, e.now) {
					m.transition(StateReeling)
				} else if m.biteDetector.TargetLostHeuristic() {
					m.transition(StateCasting)
				}
			}
		case evtFocusAcquired:
			if m.state == StateWaitingFocus {
				m.transition(StateSearching)
			}
		case evtAwaitFocus:
			if m.state == StateHalt {
				m.transition(StateWaitingFocus)
			}
		case evtForceCast:
			if m.state != StateCasting {
				m.transition(StateCasting)
			}
		case evtCancel:
			m.cooldownUntil = time.Time{}
		}
	}
	m.closed = true
}

// transition performs side effects then updates the state.
func (m *FishingStateMachine) transition(next FishingState) {
	prev := m.state
	if prev == next {
		return
	}

	switch next {
	case StateCasting:
		if m.cfg != nil {
			vk := parseVK(m.cfg.ReelKey)
			go func() {
				defer func() {
					if r := recover(); r != nil && m.logger != nil {
						m.logger.Error("cast goroutine panic", "error", r)
					}
				}()
				pressKey(vk)
			}()
			if m.logger != nil {
				m.logger.Info("cast action executed", "key", m.cfg.ReelKey)
			}
		}
		// Immediately move to searching; casting is ephemeral.
		next = StateSearching
	case StateReeling:
		if m.coordSet {
			cx, cy := m.coordX, m.coordY
			go func(x, y int) {
				defer func() {
					if r := recover(); r != nil && m.logger != nil {
						m.logger.Error("reel goroutine panic", "error", r)
					}
				}()
				moveCursor(x, y)
				time.Sleep(300 * time.Millisecond)
				clickRight()
				if m.logger != nil {
					m.logger.Info("reel action executed", "x", x, "y", y)
				}
			}(cx, cy)
		} else if m.logger != nil {
			m.logger.Info("reel action skipped - no target coords")
		}
		// Set cooldown and immediately advance to cooldown state so we don't remain stuck in reeling.
		m.cooldownUntil = time.Now().Add(m.cooldownDuration + 500*time.Millisecond)
		next = StateCooldown
	case StateCooldown:
		if m.cooldownUntil.IsZero() {
			m.cooldownUntil = time.Now().Add(m.cooldownDuration)
		}
	case StateMonitoring:
		if m.coordSet {
			cx, cy := m.coordX, m.coordY
			go func(x, y int) {
				moveCursor(x, y)
				if m.logger != nil {
					m.logger.Info("found blobber", "x", x, "y", y)
				}
			}(cx, cy)
		}
		m.biteDetector = NewBiteDetector(m.cfg, m.logger)
		m.biteDetector.Reset()
	case StateHalt:
		// no-op side effects
	}

	m.state = next
	if m.state == StateSearching {
		m.searchStarted = time.Now()
	}
	if m.logger != nil {
		m.logger.Debug("fishing state transition", "from", prev.String(), "to", next.String())
	}
	for _, l := range m.listeners {
		l(prev, next)
	}
}

func (m *FishingStateMachine) handleTick(now time.Time) {
	if m.state == StateSearching && !m.searchStarted.IsZero() && now.Sub(m.searchStarted) > 5*time.Second {
		m.transition(StateCasting)
		return
	}
	if m.state == StateCooldown && !m.cooldownUntil.IsZero() && now.After(m.cooldownUntil) {
		m.transition(StateCasting)
	}
}

// Exported API (event senders)
func (m *FishingStateMachine) AddListener(l FishingStateListener) { m.events <- evtAddListener{l: l} }
func (m *FishingStateMachine) Current() FishingState              { return m.state }
func (m *FishingStateMachine) EventTargetAcquired()               { m.events <- evtTargetAcquired{} }
func (m *FishingStateMachine) EventTargetAcquiredAt(x, y int) {
	m.events <- evtTargetAcquiredAt{x: x, y: y}
}
func (m *FishingStateMachine) EventTargetLost()   { m.events <- evtTargetLost{} }
func (m *FishingStateMachine) EventHalt()         { m.events <- evtHalt{} }
func (m *FishingStateMachine) EventFishBite()     { m.events <- evtFishBite{} }
func (m *FishingStateMachine) EventFocusAquired() { m.events <- evtFocusAcquired{} }
func (m *FishingStateMachine) EventAwaitFocus()   { m.events <- evtAwaitFocus{} }
func (m *FishingStateMachine) ForceCast()         { m.events <- evtForceCast{} }
func (m *FishingStateMachine) Cancel()            { m.events <- evtCancel{} }
func (m *FishingStateMachine) Tick(now time.Time) { m.events <- evtTick{now: now} }

// ProcessMonitoringFrame enqueues a monitoring ROI frame for bite/loss evaluation.
func (m *FishingStateMachine) ProcessMonitoringFrame(roiImg *image.RGBA, now time.Time) {
	if roiImg == nil {
		return
	}
	m.events <- evtMonitoringFrame{roi: roiImg, now: now}
}

// TargetCoordinates returns last stored target coordinates (snapshot).
func (m *FishingStateMachine) TargetCoordinates() (x, y int, ok bool) {
	if !m.coordSet {
		return 0, 0, false
	}
	return m.coordX, m.coordY, true
}

// Close stops the event loop. Further events are ignored.
func (m *FishingStateMachine) Close() {
	if m.closed {
		return
	}
	close(m.events)
}
