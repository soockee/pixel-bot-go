package fishing

import (
	"image"
	"log/slog"
	"time"

	"github.com/soocke/pixel-bot-go/config"
)

// FishingState enumerates finite states of the fishing cycle.
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

// ActionCallbacks externalize OS interactions (casting, cursor moves, reel click).
type ActionCallbacks struct {
	PressKey   func(vk byte)
	MoveCursor func(x, y int)
	ClickRight func()
	ParseVK    func(key string) byte
}

// FishingStateListener is called on each successful state transition.
type FishingStateListener func(prev, next FishingState)

// BiteDetectorContract minimal detector contract used by FSM.
type BiteDetectorContract interface {
	FeedFrame(*image.RGBA, time.Time) bool
	TargetLostHeuristic() bool
	Reset()
}

// DetectorFactory constructs a detector instance.
type DetectorFactory func(*config.Config, *slog.Logger) BiteDetectorContract

// Interface slices for consumers (presenters).
type FishingStateSource interface{ Current() FishingState }
type FishingMonitorFrame interface{ ProcessMonitoringFrame(*image.RGBA, time.Time) }
type FishingTargetOps interface {
	EventTargetAcquired()
	EventTargetAcquiredAt(int, int)
	EventTargetLost()
	TargetCoordinates() (int, int, bool)
}
type FishingFocusControl interface {
	EventAwaitFocus()
	EventFocusAcquired()
}
type FishingLifecycle interface {
	EventHalt()
	Close()
}
type FishingCasting interface {
	ForceCast()
	Cancel()
}

// FishingFSMContract aggregate for DI.
type FishingFSMContract interface {
	FishingStateSource
	FishingMonitorFrame
	FishingTargetOps
	FishingFocusControl
	FishingLifecycle
	FishingCasting
	AddListener(FishingStateListener)
}
