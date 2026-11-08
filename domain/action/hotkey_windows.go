package action

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmHotkey      = 0x0312
	modNoRepeat   = 0x4000
	hotkeyID      = 1
	whMouseLL     = 14
	wmXButtonDown = 0x020B
	wmXButtonUp   = 0x020C
	llmhfInjected = 0x01
)

var (
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procGetMessage       = user32.NewProc("GetMessageW")
	procSetWindowsHookEx = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32.NewProc("CallNextHookEx")
)

type hotkeyStart struct {
	threadID uint32
	err      error
}

// WatchHotkey calls fn each time vk is pressed, in any application, until the
// process exits. The key no longer reaches other applications. vk can be a
// mouse side button (see ParseVK). It returns an error when the key cannot be
// registered.
func WatchHotkey(vk byte, fn func()) error {
	started := make(chan hotkeyStart, 1)
	go watchHotkey(vk, fn, started)
	return (<-started).err
}

func watchHotkey(vk byte, fn func(), started chan<- hotkeyStart) {
	// Windows posts the hotkey, and calls a mouse hook, on the thread that
	// registered it, while that thread waits for messages.
	runtime.LockOSThread()
	if err := register(vk, fn); err != nil {
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

func register(vk byte, fn func()) error {
	if b := xButton(vk); b != 0 {
		return hookMouseButton(b, fn)
	}
	if r, _, err := procRegisterHotKey.Call(0, hotkeyID, modNoRepeat, uintptr(vk)); r == 0 {
		return err
	}
	return nil
}

// mouseHotkey is the one side button watched with a low-level mouse hook,
// because RegisterHotKey takes keyboard keys only.
var mouseHotkey struct {
	sync.Mutex
	button uint32 // 1 or 2; 0 when no hook is installed
	fn     func()
}

var mouseHookCallback = windows.NewCallback(mouseHookProc)

func hookMouseButton(button uint32, fn func()) error {
	mouseHotkey.Lock()
	defer mouseHotkey.Unlock()
	if mouseHotkey.button != 0 {
		return errors.New("a mouse button is already watched")
	}
	h, _, err := procSetWindowsHookEx.Call(whMouseLL, mouseHookCallback, 0, 0)
	if h == 0 {
		return err
	}
	mouseHotkey.button, mouseHotkey.fn = button, fn
	return nil
}

// msllHookStruct is the Win32 MSLLHOOKSTRUCT.
type msllHookStruct struct {
	pt        [2]int32
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

// mouseHookProc calls the hotkey's fn when its side button goes down, and
// keeps that button from other applications. Input the bot sends itself is
// passed on untouched.
func mouseHookProc(code int, wParam uintptr, info *msllHookStruct) uintptr {
	if code == 0 && (wParam == wmXButtonDown || wParam == wmXButtonUp) {
		mouseHotkey.Lock()
		button, fn := mouseHotkey.button, mouseHotkey.fn
		mouseHotkey.Unlock()
		if info.flags&llmhfInjected == 0 && info.mouseData>>16 == button && button != 0 {
			if wParam == wmXButtonDown {
				fn()
			}
			return 1
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, uintptr(unsafe.Pointer(info)))
	return r
}
