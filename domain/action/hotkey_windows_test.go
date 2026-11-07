package action

import (
	"testing"
	"time"
)

// vkF24 is a key no keyboard has, so the test takes nothing from a running game.
const vkF24 = 0x87

func TestWatchHotkeyCallsFnAndRejectsDuplicates(t *testing.T) {
	const wmQuit = 0x0012
	post := user32.NewProc("PostThreadMessageW")
	pressed := make(chan struct{}, 1)
	started := make(chan hotkeyStart, 1)

	go watchHotkey(vkF24, func() { pressed <- struct{}{} }, started)
	s := <-started
	if s.err != nil {
		t.Skipf("hotkey unavailable: %v", s.err)
	}
	defer post.Call(uintptr(s.threadID), wmQuit, 0, 0)

	if err := WatchHotkey(vkF24, func() {}); err == nil {
		t.Error("second registration of the same key: want an error")
	}

	// A posted message stands in for the key press, so no input is injected.
	if r, _, err := post.Call(uintptr(s.threadID), wmHotkey, hotkeyID, 0); r == 0 {
		t.Fatal(err)
	}
	select {
	case <-pressed:
	case <-time.After(time.Second):
		t.Fatal("fn was not called")
	}
}
