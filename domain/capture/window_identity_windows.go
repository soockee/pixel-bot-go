package capture

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindowEx  = user32.NewProc("FindWindowExW")
	procWindowVisible = user32.NewProc("IsWindowVisible")
)

// ErrFrameUnavailable marks a temporary gap in window capture.
var ErrFrameUnavailable = errors.New("capture: window frame unavailable")

// WindowIdentity binds capture and input to one window and its owning process.
type WindowIdentity struct {
	Handle windows.HWND
	PID    uint32
	Title  string
}

// ResolveWindow resolves a title once and refuses to guess between duplicates.
func ResolveWindow(title string) (WindowIdentity, error) {
	if title == "" {
		return WindowIdentity{}, errors.New("select a game window")
	}
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return WindowIdentity{}, err
	}
	var match uintptr
	for next := uintptr(0); ; {
		next, _, _ = procFindWindowEx.Call(0, next, 0, uintptr(unsafe.Pointer(name)))
		if next == 0 {
			break
		}
		visible, _, _ := procWindowVisible.Call(next)
		if visible == 0 {
			continue
		}
		if match != 0 {
			return WindowIdentity{}, fmt.Errorf("multiple windows are titled %q; close the duplicate and select again", title)
		}
		match = next
	}
	if match == 0 {
		return WindowIdentity{}, fmt.Errorf("no visible window titled %q", title)
	}
	w := WindowIdentity{Handle: windows.HWND(match), Title: title}
	if _, err := windows.GetWindowThreadProcessId(w.Handle, &w.PID); err != nil || w.PID == 0 {
		return WindowIdentity{}, errors.New("could not identify the selected game process")
	}
	return w, nil
}

// Valid checks that the handle still belongs to the process selected at start.
func (w WindowIdentity) Valid() bool {
	if w.Handle == 0 || w.PID == 0 {
		return false
	}
	var pid uint32
	_, err := windows.GetWindowThreadProcessId(w.Handle, &pid)
	return err == nil && pid == w.PID
}

// Focused checks the exact foreground handle and its process identity.
func (w WindowIdentity) Focused() bool {
	return windows.GetForegroundWindow() == w.Handle && w.Valid()
}
