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

// The hook procedure is called directly with a made-up event, so no input is
// injected and no hook has to be installed.
func TestMouseHotkeyCallsFnOnItsButtonOnly(t *testing.T) {
	pressed := 0
	mouseHotkey.Lock()
	mouseHotkey.button, mouseHotkey.fn = 2, func() { pressed++ }
	mouseHotkey.Unlock()
	t.Cleanup(func() {
		mouseHotkey.Lock()
		mouseHotkey.button, mouseHotkey.fn = 0, nil
		mouseHotkey.Unlock()
	})
	event := func(button, flags uint32, msg uintptr) uintptr {
		info := msllHookStruct{mouseData: button << 16, flags: flags}
		return mouseHookProc(0, msg, &info)
	}
	if event(2, 0, wmXButtonDown) != 1 || event(2, 0, wmXButtonUp) != 1 || pressed != 1 {
		t.Errorf("Mouse 5: pressed=%d, want 1 and both events kept from other apps", pressed)
	}
	event(1, 0, wmXButtonDown)             // Mouse 4 is not the hotkey
	event(2, llmhfInjected, wmXButtonDown) // the bot's own click
	if pressed != 1 {
		t.Errorf("pressed=%d after other buttons and injected input, want 1", pressed)
	}
	if err := hookMouseButton(1, func() {}); err == nil {
		t.Error("a second mouse hotkey: want an error")
	}
}
