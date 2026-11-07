package fishing

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

	// ObserveCast and ObserveReel run when a cast or reel is scheduled.
	// Each can return a callback to observe its eventual dispatch.
	ObserveCast func() func()
	ObserveReel func() func()
}

// FishingStateListener is invoked on state transitions.
type FishingStateListener func(prev, next FishingState)
