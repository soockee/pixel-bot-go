package action

import (
	"errors"
	"strconv"
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

const (
	vkXButton1 = 0x05 // Mouse 4
	vkXButton2 = 0x06 // Mouse 5
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

// ParseVK converts a key name (F1 to F12, a letter, a digit, Mouse4 or Mouse5)
// into a Windows virtual-key code. It reports false for any other name.
func ParseVK(key string) (byte, bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	switch k {
	case "MOUSE4":
		return vkXButton1, true
	case "MOUSE5":
		return vkXButton2, true
	}
	if len(k) == 1 && (k[0] >= 'A' && k[0] <= 'Z' || k[0] >= '0' && k[0] <= '9') {
		return k[0], true // letters and top-row digits match their VK codes
	}
	if num, ok := strings.CutPrefix(k, "F"); ok {
		if n, err := strconv.ParseUint(num, 10, 8); err == nil && n >= 1 && n <= 12 {
			return byte(0x70 + n - 1), true // VK_F1 = 0x70
		}
	}
	return 0, false
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
