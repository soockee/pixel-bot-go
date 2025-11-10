package fishing

import (
	"image"
	"log/slog"
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
	ParseVK    func(key string) byte
}

// FishingStateListener is invoked on state transitions.
type FishingStateListener func(prev, next FishingState)

// BiteDetectorContract is the interface for bite detectors.
type BiteDetectorContract interface {
	FeedFrame(*image.RGBA, time.Time) bool
	TargetLostHeuristic() bool
	Reset()
}

// DetectorFactory creates a BiteDetectorContract.
type DetectorFactory func(*config.Config, *slog.Logger) BiteDetectorContract

// Small interfaces used by consumers.
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

// FishingFSMContract aggregates the FSM API.
type FishingFSMContract interface {
	FishingStateSource
	FishingMonitorFrame
	FishingTargetOps
	FishingFocusControl
	FishingLifecycle
	FishingCasting
	AddListener(FishingStateListener)
}
