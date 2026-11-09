package capture

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procFindWindowEx  = user32.NewProc("FindWindowExW")
	procGetWindow     = user32.NewProc("GetWindow")
	procGetWindowRect = user32.NewProc("GetWindowRect")
	procWindowVisible = user32.NewProc("IsWindowVisible")
	// GetWindowDisplayAffinity tells a window kept out of screen captures, such as Go Fish's overlay.
	procDisplayAffinity = user32.NewProc("GetWindowDisplayAffinity")
)

// ErrFrameUnavailable marks a recording frame skipped to protect privacy.
var ErrFrameUnavailable = errors.New("selected window is not ready for a private recording frame")

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

// RecordingClear checks that the selected window still covers the recording
// region and that no visible top-level window above it overlaps that region.
func (w WindowIdentity) RecordingClear(area image.Rectangle) bool {
	if !w.Focused() {
		return false
	}
	current, err := w.Area()
	if err != nil || current != area {
		return false
	}
	defer physicalPixels()()
	const above = 3 // GW_HWNDPREV
	hwnd := uintptr(w.Handle)
	for range 256 {
		hwnd, _, _ = procGetWindow.Call(hwnd, above)
		if hwnd == 0 {
			return true
		}
		visible, _, _ := procWindowVisible.Call(hwnd)
		if visible == 0 {
			continue
		}
		if excludedFromCapture(hwnd) {
			continue // captures never show it, so it hides nothing of the game
		}
		var rect struct{ Left, Top, Right, Bottom int32 }
		ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		if ok == 0 || area.Overlaps(image.Rect(int(rect.Left), int(rect.Top), int(rect.Right), int(rect.Bottom))) {
			return false
		}
	}
	return false
}

// excludedFromCapture reports whether hwnd is kept out of screen captures
// (WDA_EXCLUDEFROMCAPTURE), like Go Fish's overlay over the game.
func excludedFromCapture(hwnd uintptr) bool {
	const excludeFromCapture = 0x11
	var affinity uint32
	ok, _, _ := procDisplayAffinity.Call(hwnd, uintptr(unsafe.Pointer(&affinity)))
	return ok != 0 && affinity == excludeFromCapture
}
