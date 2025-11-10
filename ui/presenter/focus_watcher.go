package presenter

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FocusFSM is the minimal FSM interface used by FocusWatcher.
type FocusFSM interface {
	Current() fishing.FishingState
	EventFocusAcquired()
}

// FocusWatcher watches the foreground window while the FSM is in
// StateWaitingFocus and triggers EventFocusAcquired when a match occurs.
type FocusWatcher struct {
	FSM        FocusFSM
	Logger     *slog.Logger
	Foreground func() (string, error)
	Selected   func() string
	interval   time.Duration
	running    atomic.Bool
	done       chan struct{}
	fired      bool
	lastTitle  string
}

// NewFocusWatcher returns a FocusWatcher. If fg or sel are nil, defaults are used.
func NewFocusWatcher(fsm FocusFSM, logger *slog.Logger, fg func() (string, error), sel func() string) *FocusWatcher {
	if fg == nil {
		fg = action.ForegroundWindowTitle
	}
	if sel == nil {
		sel = func() string { return "" }
	}
	return &FocusWatcher{FSM: fsm, Logger: logger, Foreground: fg, Selected: sel, interval: 250 * time.Millisecond}
}

// OnState starts polling when next == StateWaitingFocus and stops otherwise.
// Register it as an FSM listener.
func (w *FocusWatcher) OnState(prev, next fishing.FishingState) {
	if w == nil {
		return
	}
	if next == fishing.StateWaitingFocus {
		w.start()
		return
	}
	// Stop polling when leaving the waiting state.
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
	// Ensure FSM is still waiting for focus.
	if w.FSM.Current() != fishing.StateWaitingFocus {
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
	// Only react on title change.
	if current != w.lastTitle {
		w.lastTitle = current
		if current == selected {
			w.FSM.EventFocusAcquired()
			w.fired = true
			if w.Logger != nil {
				w.Logger.Debug("focus acquired", "window", fgTitle)
			}
			w.stop()
		}
	}
}

func (w *FocusWatcher) reset() {
	w.fired = false
	w.lastTitle = ""
}
