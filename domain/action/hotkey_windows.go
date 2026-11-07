package action

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmHotkey    = 0x0312
	modNoRepeat = 0x4000
	hotkeyID    = 1
)

var (
	procRegisterHotKey = user32.NewProc("RegisterHotKey")
	procGetMessage     = user32.NewProc("GetMessageW")
)

type hotkeyStart struct {
	threadID uint32
	err      error
}

// WatchHotkey calls fn on its own goroutine each time vk is pressed, in any
// application, until the process exits. The key no longer reaches other
// applications. It returns an error when the key cannot be registered.
func WatchHotkey(vk byte, fn func()) error {
	started := make(chan hotkeyStart, 1)
	go watchHotkey(vk, fn, started)
	return (<-started).err
}

func watchHotkey(vk byte, fn func(), started chan<- hotkeyStart) {
	// Windows posts the hotkey to the thread that registered it.
	runtime.LockOSThread()
	if r, _, err := procRegisterHotKey.Call(0, hotkeyID, modNoRepeat, uintptr(vk)); r == 0 {
		started <- hotkeyStart{err: err}
		return
	}
	started <- hotkeyStart{threadID: windows.GetCurrentThreadId()}

	var msg struct {
		hwnd    uintptr
		message uint32
		wParam  uintptr
		lParam  uintptr
		time    uint32
		pt      [2]int32
		_       uint32
	}
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT or an error
			return
		}
		if msg.message == wmHotkey {
			fn()
		}
	}
}
