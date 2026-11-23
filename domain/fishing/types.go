package fishing

import (
	"errors"
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

// Input is all input the cycle sends to the game. The live one goes to the
// Board through action.Guard, a dry run's only to the log (docs/adr/0001).
type Input interface {
	// Tap presses vk for hold, or for the Board's default hold when it is zero.
	Tap(vk byte, hold time.Duration) error
	// MoveTo puts the pointer on p, or says why it did not get there.
	MoveTo(p image.Point) error
	// ClickRight clicks the bobber, or says why it could not.
	ClickRight() error
	// DragCamera turns the camera by dx. attempted is false when no drag fits,
	// so the caller walks instead.
	DragCamera(dx int) (attempted, moved bool)
}

// ActionCallbacks exposes OS actions used by the fishing logic.
type ActionCallbacks struct {
	// Input is required: every key, click and pointer move goes through it.
	Input Input
	// CursorPos reads the pointer, where a between-catch wander starts.
	CursorPos func() (x, y int, ok bool)
	ParseVK   func(key string) (byte, bool)
	// CanAct checks focus just before dispatching an optional action.
	CanAct func() bool
	// OpenBoard validates and opens the Board for one run.
	OpenBoard func(config.Config) (closeBoard func(), err error)

	// ObserveCast and ObserveReel run when a cast or reel is scheduled.
	// Each can return a callback to observe its eventual dispatch.
	ObserveCast func() func()
	ObserveReel func() func()

	// ReelRoll optionally supplies a pseudorandom draw in [0, n) for reel variation.
	ReelRoll func(n int) int
	// RepositionRoll varies live settle times and camera distances.
	RepositionRoll func(n int) int
	// Approach selects a waiting position after the locator locks.
	Approach func(center image.Point, box image.Rectangle) (image.Point, bool)

	// PlanReel schedules a timed hold and cooldown. A zero cooldown keeps the
	// configured value. A reel without the Board simulates the hold.
	PlanReel          func(biteDetector string) ReelTiming
	ObserveReelTiming func(ReelTimingEvent)
	CanSimulateReel   func() bool
	Cursor            *CursorCallbacks
}

// CursorStep places the pointer at Point, After time has elapsed in a plan.
type CursorStep struct {
	After time.Duration
	Point image.Point
}

// ErrPointerMissed is a Cursor.Move error for a pointer that still works but
// did not land on the point. The FSM gives up that cast instead of halting.
var ErrPointerMissed = errors.New("the pointer did not land on the bobber")

// CursorCallbacks plans a path that the FSM loop moves along with
// Input.MoveTo. False from Plan means the app cannot safely take control of
// the pointer.
type CursorCallbacks struct {
	Plan  func(target image.Point, settle bool) ([]CursorStep, bool)
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
