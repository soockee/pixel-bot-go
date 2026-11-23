package presenter

import (
	"context"
	"image"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FocusFSM is the minimal FSM interface used by FocusWatcher.
type FocusFSM interface {
	Current() fishing.FishingState
	PausedByMouse() bool
	EventFocusAcquired()
	EventFocusLost()
	EventMouseSettled()
}

// FocusWatcher polls the selected window while a run is active and tells the
// FSM when it gains or loses focus, and, while the person's mouse paused the
// run, when the mouse has rested in the game.
type FocusWatcher struct {
	fsm      FocusFSM
	focused  func() bool
	pointer  func() (at image.Point, inGame bool)
	interval time.Duration
	rest     time.Duration // how long the mouse stays still before the run goes on
	mu       sync.Mutex
	cancel   context.CancelFunc // non-nil while polling

	// Owned by the polling goroutine: where the pointer was, and since when.
	lastAt image.Point
	still  time.Time
}

// NewFocusWatcher returns a FocusWatcher. focused reports whether the selected
// window is in front, pointer where the OS pointer is and whether that is
// inside the game window.
func NewFocusWatcher(fsm FocusFSM, focused func() bool, pointer func() (image.Point, bool)) *FocusWatcher {
	return &FocusWatcher{fsm: fsm, focused: focused, pointer: pointer, interval: 250 * time.Millisecond, rest: 2 * time.Second}
}

// OnState polls in every state except halt. Register it as an FSM listener.
func (w *FocusWatcher) OnState(_, next fishing.FishingState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if next == fishing.StateHalt {
		if w.cancel != nil {
			w.cancel()
			w.cancel = nil
		}
		return
	}
	if w.cancel == nil {
		var ctx context.Context
		ctx, w.cancel = context.WithCancel(context.Background())
		go w.loop(ctx)
	}
}

func (w *FocusWatcher) loop(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *FocusWatcher) poll() {
	focused := w.focused()
	switch w.fsm.Current() {
	case fishing.StateHalt:
	case fishing.StateWaitingFocus:
		switch {
		case !focused:
			w.still = time.Time{}
		case !w.fsm.PausedByMouse():
			w.fsm.EventFocusAcquired()
		default:
			w.watchRest()
		}
	default:
		if !focused {
			w.fsm.EventFocusLost()
		}
	}
}

// watchRest sends EventMouseSettled once the pointer has stayed on one point
// inside the game window for w.rest.
func (w *FocusWatcher) watchRest() {
	at, inGame := w.pointer()
	now := time.Now()
	if !inGame || at != w.lastAt || w.still.IsZero() {
		w.lastAt, w.still = at, now
		if !inGame {
			w.still = time.Time{}
		}
		return
	}
	if now.Sub(w.still) >= w.rest {
		w.still = time.Time{}
		w.fsm.EventMouseSettled()
	}
}
