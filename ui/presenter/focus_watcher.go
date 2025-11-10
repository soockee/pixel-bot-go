package presenter

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FocusFSM narrows the FSM contract needed by the focus watcher.
type FocusFSM interface {
	Current() fishing.FishingState
	EventFocusAcquired()
}

// FocusWatcher automatically watches which window is in focus and fire the event on window change.
type FocusWatcher struct {
	FSM        FocusFSM
	Logger     *slog.Logger
	Foreground func() (string, error)
	Selected   func() string // user-selected window title (normalized by provider)
	interval   time.Duration
	running    atomic.Bool
	done       chan struct{}
	fired      bool
	lastTitle  string // last foreground title seen (normalized)
}

// NewFocusWatcher constructs a focus watcher with optional delay.
func NewFocusWatcher(fsm FocusFSM, logger *slog.Logger, fg func() (string, error), sel func() string) *FocusWatcher {
	if fg == nil {
		fg = action.ForegroundWindowTitle
	}
	if sel == nil {
		sel = func() string { return "" }
	}
	return &FocusWatcher{FSM: fsm, Logger: logger, Foreground: fg, Selected: sel, interval: 250 * time.Millisecond}
}

// Tick checks state and triggers EventFocusAcquired after Delay in waiting state.
// OnState should be called from an FSM listener; it starts/stops polling based on WaitingFocus state.
func (w *FocusWatcher) OnState(prev, next fishing.FishingState) {
	if w == nil {
		return
	}
	if next == fishing.StateWaitingFocus {
		w.start()
		return
	}
	// leaving waiting state
	w.stop()
}

func (w *FocusWatcher) start() {
	if w.running.Load() {
		return
	}
	w.done = make(chan struct{})
	w.running.Store(true)
	w.fired = false
	w.lastTitle = ""
	go w.loop()
}

func (w *FocusWatcher) stop() {
	if !w.running.Load() {
		return
	}
	close(w.done)
	w.running.Store(false)
}

func (w *FocusWatcher) loop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.poll()
		case <-w.done:
			return
		}
	}
}

func (w *FocusWatcher) poll() {
	if w.fired || w.FSM == nil {
		return
	}
	if w.FSM.Current() != fishing.StateWaitingFocus { // safety
		return
	}
	if w.Foreground == nil {
		w.Foreground = action.ForegroundWindowTitle
	}
	fgTitle, err := w.Foreground()
	if err != nil {
		w.Logger.Error("foreground title error", "error", err)
		return
	}
	fgTitle = strings.TrimSpace(fgTitle)
	if fgTitle == "" {
		return
	}
	selected := strings.TrimSpace(strings.ToLower(w.Selected()))
	if selected == "" {
		return
	}
	current := strings.ToLower(fgTitle)
	if current != w.lastTitle { // only react on change
		w.lastTitle = current
		if current == selected {
			w.FSM.EventFocusAcquired()
			w.fired = true
			if w.Logger != nil {
				w.Logger.Debug("focus acquired", "window", fgTitle)
			}
			// stop after firing to save cycles
			w.stop()
		}
	}
}

func (w *FocusWatcher) reset() { // unused after refactor; retained for future reuse
	w.fired = false
	w.lastTitle = ""
}
