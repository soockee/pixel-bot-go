package presenter

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/soocke/pixel-bot-go/domain/fishing"
)

type mockFocusFSM struct {
	state    atomic.Int32
	acquired atomic.Int32
	lost     atomic.Int32
}

func (m *mockFocusFSM) set(s fishing.FishingState)    { m.state.Store(int32(s)) }
func (m *mockFocusFSM) Current() fishing.FishingState { return fishing.FishingState(m.state.Load()) }
func (m *mockFocusFSM) EventFocusAcquired() {
	m.acquired.Add(1)
	m.set(fishing.StateSearching)
}
func (m *mockFocusFSM) EventFocusLost() {
	m.lost.Add(1)
	m.set(fishing.StateWaitingFocus)
}

func newFastWatcher(fsm FocusFSM, focused *atomic.Bool) *FocusWatcher {
	w := NewFocusWatcher(fsm, focused.Load)
	w.interval = time.Millisecond
	return w
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFocusWatcherFollowsFocusInBothDirections(t *testing.T) {
	fsm := &mockFocusFSM{}
	fsm.set(fishing.StateWaitingFocus)
	var focused atomic.Bool
	w := newFastWatcher(fsm, &focused)
	w.OnState(fishing.StateHalt, fishing.StateWaitingFocus)
	defer w.OnState(fishing.StateSearching, fishing.StateHalt)

	time.Sleep(10 * time.Millisecond)
	if fsm.acquired.Load() != 0 {
		t.Fatal("focus acquired while the window was in the background")
	}

	focused.Store(true)
	waitFor(t, "focus acquired", func() bool { return fsm.acquired.Load() == 1 })

	focused.Store(false)
	waitFor(t, "focus lost", func() bool { return fsm.lost.Load() == 1 })

	focused.Store(true)
	waitFor(t, "focus regained", func() bool { return fsm.acquired.Load() == 2 })
}

func TestFocusWatcherIgnoresFocusChangesInHalt(t *testing.T) {
	fsm := &mockFocusFSM{}
	fsm.set(fishing.StateHalt)
	var focused atomic.Bool
	focused.Store(true)
	w := newFastWatcher(fsm, &focused)

	w.poll()

	if fsm.acquired.Load() != 0 || fsm.lost.Load() != 0 {
		t.Error("watcher acted on a halted FSM")
	}
}

func TestFocusWatcherStopsOnHalt(t *testing.T) {
	fsm := &mockFocusFSM{}
	fsm.set(fishing.StateSearching)
	var focused atomic.Bool
	focused.Store(true)
	w := newFastWatcher(fsm, &focused)
	w.OnState(fishing.StateHalt, fishing.StateSearching)

	w.OnState(fishing.StateSearching, fishing.StateHalt)
	time.Sleep(5 * time.Millisecond) // lets a poll already running finish
	focused.Store(false)
	time.Sleep(10 * time.Millisecond)

	if fsm.lost.Load() != 0 {
		t.Error("watcher kept polling after halt")
	}
}
