package app

import (
	"log/slog"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// FishingState represents the high-level finite states of the fishing cycle.
type FishingState int

const (
	StateSearching  FishingState = iota // searching for the target object
	StateMonitoring                     // monitoring target for bite movement
	StateReeling                        // performing reel action (mouse press)
	StateCooldown                       // waiting for loot/timeout before next cast
	StateCasting                        // casting the fishing rod (key press)
)

func (s FishingState) String() string {
	switch s {
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
	default:
		return "unknown"
	}
}

// FishingStateListener is invoked on every successful state transition.
type FishingStateListener func(prev, next FishingState)

// FishingStateMachine coordinates transitions between fishing states.
// It is concurrency-safe; external events may call its exported methods
// from any goroutine.
type FishingStateMachine struct {
	mu        sync.Mutex
	state     FishingState
	logger    *slog.Logger
	listeners []FishingStateListener
	cfg       *config.Config

	// Configuration
	cooldownDuration time.Duration // time spent in cooldown before recast

	// Internal timing bookkeeping
	cooldownUntil time.Time // zero if not in cooldown
	searchStarted time.Time // time when we entered searching (for auto-recast timeout)

	// Target coordinate captured when entering monitoring (acquired) state.
	coordX   int
	coordY   int
	coordSet bool
}

// NewFishingStateMachine creates a new machine starting in StateSearching.
func NewFishingStateMachine(logger *slog.Logger, cfg *config.Config) *FishingStateMachine {
	cooldown := time.Duration(1) * time.Second
	if cfg != nil && cfg.CooldownSeconds > 0 {
		cooldown = time.Duration(cfg.CooldownSeconds) * time.Second
	}
	return &FishingStateMachine{
		state:            StateSearching,
		logger:           logger,
		cfg:              cfg,
		cooldownDuration: cooldown,
		searchStarted:    time.Now(),
	}
}

// AddListener registers a listener for state transitions.
func (m *FishingStateMachine) AddListener(l FishingStateListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, l)
}

// Current returns the current state.
func (m *FishingStateMachine) Current() FishingState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// transition performs the state change if valid and runs side effects.
func (m *FishingStateMachine) transition(next FishingState) {
	prev := m.state
	if prev == next {
		return
	}
	// Determine final state after applying any ephemeral entry actions.
	final := next
	ephemeralPerformed := false
	switch next {
	case StateCasting:
		// Casting: press configured key (reusing ReelKey until dedicated cast key exists).
		if m.cfg != nil {
			vk := parseVK(m.cfg.ReelKey)
			pressKey(vk)
			if m.logger != nil {
				m.logger.Info("cast action executed", "key", m.cfg.ReelKey)
			}
		}
		final = StateSearching
		ephemeralPerformed = true
	case StateReeling:
		// Reeling: move cursor to stored target; wait 0.5s; right-click, then press key.
		if m.coordSet {
			cx, cy := m.coordX, m.coordY
			go func(x, y int) {
				moveCursor(x, y)
				time.Sleep(300 * time.Millisecond) // requested delay before right-click
				clickRight()
				if m.logger != nil {
					m.logger.Info("reel action executed", "x", x, "y", y, "delay_ms", 300)
				}
			}(cx, cy)
		} else if m.logger != nil {
			m.logger.Info("reel action skipped - no target coords")
		}
		// Include the delay in cooldown timing so overall cycle accounts for it.
		m.cooldownUntil = time.Now().Add(m.cooldownDuration + 500*time.Millisecond)
		final = StateCooldown
		ephemeralPerformed = true
	case StateCooldown:
		if m.cooldownUntil.IsZero() {
			m.cooldownUntil = time.Now().Add(m.cooldownDuration)
		}
	case StateMonitoring:
		// Monitoring has no immediate entry side-effects here.
	}
	// Commit final state.
	m.state = final
	if final == StateSearching { // record start of searching period for timeout logic
		m.searchStarted = time.Now()
	}
	if m.logger != nil {
		if ephemeralPerformed {
			m.logger.Debug("fishing state transition", "from", prev.String(), "via", next.String(), "to", final.String())
		} else {
			m.logger.Debug("fishing state transition", "from", prev.String(), "to", final.String())
		}
	}
	// Single listener notification (prev -> final). Ephemeral intermediate states are not emitted.
	for _, l := range m.listeners {
		l(prev, final)
	}
}

// EventTargetAcquired should be called when the bobber/target becomes detectable.
func (m *FishingStateMachine) EventTargetAcquired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateSearching {
		m.transition(StateMonitoring)
	}
}

// EventTargetAcquiredAt stores the coordinates and transitions to monitoring.
func (m *FishingStateMachine) EventTargetAcquiredAt(x, y int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coordX, m.coordY, m.coordSet = x, y, true
	if m.state == StateSearching {
		m.transition(StateMonitoring)
	}
}

// EventTargetLost can be called when tracking lost (optional fallback).
func (m *FishingStateMachine) EventTargetLost() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Only revert to searching if we were monitoring.
	if m.state == StateMonitoring {
		m.transition(StateCasting)
	}
}

// EventFishBite indicates movement/bite detected while monitoring.
func (m *FishingStateMachine) EventFishBite() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateMonitoring {
		m.transition(StateReeling)
	}
}

// ForceCast allows external code to initiate a cast (e.g. user command) from any state except when already casting.
func (m *FishingStateMachine) ForceCast() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != StateCasting {
		m.transition(StateCasting)
	}
}

// Cancel stops timers and leaves machine in its current state.
func (m *FishingStateMachine) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cooldownUntil = time.Time{}
}

// Reset forces machine back to searching (clearing timers) without performing actions.
func (m *FishingStateMachine) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cooldownUntil = time.Time{}
	m.coordSet = false
	m.transition(StateSearching)
}

// Tick should be called periodically (e.g. from app.update) to advance cooldown.
func (m *FishingStateMachine) Tick(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Auto-recast if searching has exceeded hard timeout (5s) without acquiring target.
	if m.state == StateSearching && !m.searchStarted.IsZero() && now.Sub(m.searchStarted) > 5*time.Second {
		m.transition(StateCasting)
		return
	}
	if m.state == StateCooldown && !m.cooldownUntil.IsZero() && now.After(m.cooldownUntil) {
		m.transition(StateCasting)
	}
}

// TargetCoordinates returns last stored target coordinates.
func (m *FishingStateMachine) TargetCoordinates() (x, y int, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.coordSet {
		return 0, 0, false
	}
	return m.coordX, m.coordY, true
}

// SetMaxCastDuration allows external code (app) to override the monitoring timeout based on config.
// SetMaxCastDuration removed; timeout handled exclusively by BiteDetector heuristic.
