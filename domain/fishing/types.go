package fishing

import (
	"errors"
	"image"
	"time"
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

// Deps are what the FSM needs from outside: the Input, the Pointer and the
// Observer, the focus check, the dice and the reel plan. Every field is
// required or a plain switch, so the FSM checks none of them for nil.
type Deps struct {
	// Input is required: every key, click and pointer move goes through it.
	Input Input
	// SimulateHolds makes the FSM hold keys on its own clock, so a dry run's
	// trace shows each press and release. Without it the Board times its holds.
	SimulateHolds bool
	// Pointer is required: it plans, aims and reads the pointer that Input moves.
	Pointer Pointer
	// InFront is required: whether the game has focus, checked before optional
	// input and before a reel.
	InFront func() bool

	// Observer is required: NopObserver for a run without a trace.
	Observer Observer

	// Roll is required: a pseudorandom draw in [0, n), for every pause, idle
	// move, reposition and deliberate miss the cycle varies.
	Roll func(n int) int
	// DeliberateMisses lets one bite in 20 go: one not reeled, one clicked
	// beside the bobber. A dry run leaves it off, so a miss in its trace is
	// always a missed detection.
	DeliberateMisses bool

	// PlanReel is required: it schedules a reel's hold, cooldown and delay
	// after the bite. A zero field keeps the default. With SimulateHolds the
	// FSM holds on its own clock.
	PlanReel func(biteDetector string) ReelTiming
}

// CursorStep places the pointer at Point, After time has elapsed in a plan.
type CursorStep struct {
	After time.Duration
	Point image.Point
}

// ErrNotInFront is the Input's error for input it dropped because the game
// was not in front. The run pauses on it and continues once the game is back.
var ErrNotInFront = errors.New("the game was not in front")

// ErrPointerMissed is an Input.MoveTo error for a pointer that still works but
// did not land on the point. The FSM gives up that cast instead of halting.
var ErrPointerMissed = errors.New("the pointer did not land on the bobber")

// Observer watches the cycle for a dry run's trace.
type Observer interface {
	// Cast and Reel run when a cast or reel is planned, and return what to
	// run once it goes out.
	Cast() (dispatched func())
	Reel() (dispatched func())
	// ReelTiming sees each simulated hold planned, released or canceled.
	ReelTiming(ReelTimingEvent)
}

// NopObserver watches nothing.
type NopObserver struct{}

func (NopObserver) Cast() func()               { return func() {} }
func (NopObserver) Reel() func()               { return func() {} }
func (NopObserver) ReelTiming(ReelTimingEvent) {}

// Pointer plans the pointer paths that the FSM loop moves along with
// Input.MoveTo.
type Pointer interface {
	// Plan is the path to target. False means the app cannot safely take
	// control of the pointer.
	Plan(target image.Point, settle bool) ([]CursorStep, bool)
	// Reset forgets where the last path left the pointer, so a move by the
	// person after it is no takeover.
	Reset()
	// Approach is where to wait while watching the bobber at center in box,
	// and whether to move there at all.
	Approach(center image.Point, box image.Rectangle) (image.Point, bool)
	// Pos is where the pointer is now, where a between-catch wander starts.
	Pos() (image.Point, bool)
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
