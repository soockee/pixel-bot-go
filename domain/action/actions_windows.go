package action

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procGetCursorPos     = user32.NewProc("GetPhysicalCursorPos")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procGetWindowTxt     = user32.NewProc("GetWindowTextW")
	procIsVisible        = user32.NewProc("IsWindowVisible")
)

// LeftMouseDown reports whether the left mouse button is already held.
func LeftMouseDown() bool {
	const vkLeftButton = 0x01
	state, _, _ := procGetAsyncKeyState.Call(vkLeftButton)
	return state&0x8000 != 0
}

// CursorPos returns the current position of the OS mouse pointer.
func CursorPos() (x, y int, ok bool) {
	var p struct{ X, Y int32 }
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), r != 0
}

func windowTitle(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowTxt.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(windows.UTF16ToString(buf[:n]))
}

// ListWindows returns titles of top-level visible windows.
// Empty titles are skipped.
func ListWindows() ([]string, error) {
	var titles []string
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if visible, _, _ := procIsVisible.Call(hwnd); visible != 0 {
			if title := windowTitle(hwnd); title != "" {
				titles = append(titles, title)
			}
		}
		return 1 // continue enumeration
	})
	if err := windows.EnumWindows(cb, nil); err != nil {
		return nil, err
	}
	return titles, nil
}

// ForegroundWindowTitle returns the title of the current foreground window.
// If no foreground window is available an error is returned.
func ForegroundWindowTitle() (string, error) {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return "", errors.New("no foreground window")
	}
	return windowTitle(uintptr(hwnd)), nil
}
