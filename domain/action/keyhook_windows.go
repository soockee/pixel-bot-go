package action

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage        = user32.NewProc("GetMessageW")
	procPeekMessage       = user32.NewProc("PeekMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
)

const (
	whKeyboardLL  = 13
	wmQuit        = 0x0012
	wmKeyDown     = 0x0100
	wmSysKeyDown  = 0x0104
	llkhfInjected = 0x10
	pmNoRemove    = 0
)

// kbdLLHook is KBDLLHOOKSTRUCT.
type kbdLLHook struct {
	VKCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}

// The hook callback is made once: Windows callbacks are never freed. While
// CatchKey runs, it swallows catchVK from a real keyboard and reports its
// first press on caught.
var (
	catchMu   sync.Mutex // one CatchKey at a time
	catchVK   uint32
	caught    chan time.Time
	hookProc  uintptr
	hookOnce  sync.Once
	hookState sync.Mutex // guards catchVK and caught against the callback
)

func keyHook(code int, wParam, lParam uintptr) uintptr {
	if code >= 0 {
		// lParam points at a KBDLLHOOKSTRUCT owned by Windows for this call.
		k := *(**kbdLLHook)(unsafe.Pointer(&lParam))
		hookState.Lock()
		vk, ch := catchVK, caught
		hookState.Unlock()
		// A program's SendInput is marked injected; only a device's key counts.
		if ch != nil && k.VKCode == vk && k.Flags&llkhfInjected == 0 {
			if wParam == wmKeyDown || wParam == wmSysKeyDown {
				select {
				case ch <- time.Now():
				default:
				}
			}
			return 1 // swallowed: no window sees the test key
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

// CatchKey watches every keyboard for vk while press sends it, swallows it so
// no window gets it, and returns how long after the start of press its first
// hardware key press arrived. It fails when none arrives within wait.
func CatchKey(vk byte, wait time.Duration, press func() error) (time.Duration, error) {
	catchMu.Lock()
	defer catchMu.Unlock()
	hookOnce.Do(func() { hookProc = windows.NewCallback(keyHook) })

	ch := make(chan time.Time, 1)
	hookState.Lock()
	catchVK, caught = uint32(vk), ch
	hookState.Unlock()
	defer func() {
		hookState.Lock()
		caught = nil
		hookState.Unlock()
	}()

	// A low-level hook runs on the thread that installed it, which must pump messages.
	ready := make(chan error, 1)
	var thread uint32
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		thread = windows.GetCurrentThreadId()
		var msg [48]byte                                                            // MSG
		procPeekMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, pmNoRemove) // creates the message queue
		hook, _, err := procSetWindowsHookEx.Call(whKeyboardLL, hookProc, 0, 0)
		if hook == 0 {
			ready <- fmt.Errorf("install the key check: %w", err)
			return
		}
		defer procUnhookWindowsHook.Call(hook)
		ready <- nil
		for {
			if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0); r == 0 || int32(r) == -1 {
				return
			}
		}
	}()
	if err := <-ready; err != nil {
		return 0, err
	}
	defer procPostThreadMessage.Call(uintptr(thread), wmQuit, 0, 0)

	start := time.Now()
	if err := press(); err != nil {
		return 0, err
	}
	select {
	case at := <-ch:
		time.Sleep(200 * time.Millisecond) // the release follows the press; swallow it too
		return at.Sub(start), nil
	case <-time.After(wait):
		return 0, errors.New("the key did not arrive in Windows")
	}
}
