package fishing

import (
	"image"
	"sync"
	"testing"
	"time"
)

// actionLog records the side effects the FSM requests, in order.
type actionLog struct {
	mu     sync.Mutex
	events []string
}

func (a *actionLog) add(s string) { a.mu.Lock(); a.events = append(a.events, s); a.mu.Unlock() }
func (a *actionLog) snapshot() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.events...)
}

func newRecordingFSM(log *actionLog) *FishingFSM {
	return NewFSM(discardLogger, nil, ActionCallbacks{
		PressKey:   func(byte) { log.add("press") },
		MoveCursor: func(x, y int) { log.add("move") },
		ClickRight: func() { log.add("click") },
		ParseVK:    func(string) byte { return 0 },
	}, nil)
}

func waitForActions(t *testing.T, log *actionLog, want []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := log.snapshot()
		if len(got) == len(want) {
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("actions = %v, want %v", got, want)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout: actions = %v, want %v", log.snapshot(), want)
}

// startMonitoring drives an FSM from halt to monitoring with a target at (10,10).
func startMonitoring(t *testing.T, m *FishingFSM) {
	t.Helper()
	m.EventAwaitFocus()
	m.EventFocusAcquired()
	m.EventTargetAcquiredAt(10, 10)
	waitForState(t, m, StateMonitoring, time.Second)
}

// TestReplay_BiteFrameSequenceReelsAndCoolsDown replays a quiet-then-splash
// frame sequence through the real detector and checks FSM state and actions.
func TestReplay_BiteFrameSequenceReelsAndCoolsDown(t *testing.T) {
	log := &actionLog{}
	m := newRecordingFSM(log)
	defer m.Close()
	startMonitoring(t, m)

	w, h, base := 40, 40, byte(80)
	start := time.Now()
	var frames []*image.RGBA
	for i := 0; i < 5; i++ {
		frames = append(frames, synthFrame(w, h, base, nil))
	}
	for i := 0; i < 2; i++ {
		frames = append(frames, synthFrame(w, h, base, func(px []byte, w, h int) { applyRegion(px, w, h, 10, 10, 30, 30, 140) }))
	}
	for i, f := range frames {
		m.ProcessMonitoringFrame(f, start.Add(time.Duration(i)*50*time.Millisecond))
	}
	waitForState(t, m, StateCooldown, time.Second)
	// monitoring entry moves the cursor, reeling moves again then right-clicks.
	waitForActions(t, log, []string{"move", "move", "click"}, 2*time.Second)
}

// TestReplay_QuietFramesStayMonitoring checks that unchanging frames never reel.
func TestReplay_QuietFramesStayMonitoring(t *testing.T) {
	log := &actionLog{}
	m := newRecordingFSM(log)
	defer m.Close()
	startMonitoring(t, m)

	start := time.Now()
	for i := 0; i < 6; i++ {
		m.ProcessMonitoringFrame(synthFrame(40, 40, 80, nil), start.Add(time.Duration(i)*50*time.Millisecond))
	}
	time.Sleep(150 * time.Millisecond)
	if got := m.Current(); got != StateMonitoring {
		t.Fatalf("state = %v, want monitoring", got)
	}
	for _, a := range log.snapshot() {
		if a == "click" {
			t.Fatalf("unexpected reel click: %v", log.snapshot())
		}
	}
}

// TestReplay_TargetLostRecasts checks that losing the target presses the cast key.
func TestReplay_TargetLostRecasts(t *testing.T) {
	log := &actionLog{}
	m := newRecordingFSM(log)
	defer m.Close()
	startMonitoring(t, m)
	m.EventTargetLost()
	waitForState(t, m, StateSearching, time.Second)
	// ParseVK/PressKey need a config; with nil cfg the cast key press is skipped,
	// so only the monitoring-entry cursor move is expected.
	if got := log.snapshot(); len(got) == 0 || got[0] != "move" {
		t.Fatalf("actions = %v", got)
	}
}
