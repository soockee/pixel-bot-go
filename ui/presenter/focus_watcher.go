package presenter

import (
	"context"
	"sync"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

// FocusFSM is the minimal FSM interface used by FocusWatcher.
type FocusFSM interface {
	Current() fishing.FishingState
	EventFocusAcquired()
	EventFocusLost()
}

// FocusWatcher polls the selected window while a run is active and tells the
// FSM when it gains or loses focus.
type FocusWatcher struct {
	fsm      FocusFSM
	focused  func() bool
	interval time.Duration
	mu       sync.Mutex
	cancel   context.CancelFunc // non-nil while polling
}

// NewFocusWatcher returns a FocusWatcher. focused reports whether the selected
// window is in front.
func NewFocusWatcher(fsm FocusFSM, focused func() bool) *FocusWatcher {
	return &FocusWatcher{fsm: fsm, focused: focused, interval: 250 * time.Millisecond}
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
		if focused {
			w.fsm.EventFocusAcquired()
		}
	default:
		if !focused {
			w.fsm.EventFocusLost()
		}
	}
}
