package fishing

import (
	"image"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// FishingState enumerates fishing FSM states.
type FishingState int

const (
	StateSearching FishingState = iota
	StateMonitoring
	StateReeling
	StateCooldown
	StateCasting
	StateHalt
	StateWaitingFocus
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

// ActionCallbacks exposes OS actions used by the fishing logic.
type ActionCallbacks struct {
	PressKey   func(vk byte)
	MoveCursor func(x, y int)
	ClickRight func()
	ParseVK    func(key string) (byte, bool)
	// OpenKeyboard validates and opens the external keyboard for one run.
	OpenKeyboard func(config.Config) (closeKeyboard func(), err error)
	TapKeyboard  func(vk byte) error

	// ObserveCast and ObserveReel run when a cast or reel is scheduled.
	// Each can return a callback to observe its eventual dispatch.
	ObserveCast func() func()
	ObserveReel func() func()

	// PlanReel schedules a timed hold and cooldown. A zero cooldown keeps the
	// configured value. Without HoldRight, the button hold is simulated.
	PlanReel          func(biteDetector string) ReelTiming
	HoldRight         func() func()        // returns a mandatory release, nil if press failed
	HoldInteract      func(vk byte) func() // same contract for the game's Interact binding
	ObserveReelTiming func(ReelTimingEvent)
	CanSimulateReel   func() bool
	Cursor            *CursorCallbacks
}

// CursorStep places the pointer at Point, After time has elapsed in a plan.
type CursorStep struct {
	After time.Duration
	Point image.Point
}

// CursorCallbacks plans and executes a path on the FSM loop. False from Plan
// or Move means the app cannot safely take control of the pointer.
type CursorCallbacks struct {
	Plan  func(target image.Point, settle bool) ([]CursorStep, bool)
	Move  func(image.Point) bool
	Reset func()
}

// ReelTiming is the hold and the wait from release to the next cast.
type ReelTiming struct {
	Hold, Cooldown time.Duration
	AfterDetection time.Duration // zero keeps the fixed delay
}

// ReelTimingEvent describes a simulated reel plan, release, or cancellation.
type ReelTimingEvent struct {
	Kind, Reason string
	Detector     string
	DetectorAt   time.Time
	Timing       ReelTiming
	Delay        time.Duration
}

// FishingStateListener is invoked on state transitions.
type FishingStateListener func(prev, next FishingState)
