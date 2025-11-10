package action

import (
	"errors"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ClickRight sends a right mouse button click (down then up).
// Windows implementation using the Win32 API.
func ClickRight() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	mouseEvent := user32.NewProc("mouse_event")
	const MOUSEEVENTF_RIGHTDOWN = 0x0008
	const MOUSEEVENTF_RIGHTUP = 0x0010
	_, _, _ = mouseEvent.Call(MOUSEEVENTF_RIGHTDOWN, 0, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	_, _, _ = mouseEvent.Call(MOUSEEVENTF_RIGHTUP, 0, 0, 0, 0)
}

// MoveCursor moves the OS mouse pointer to (x, y).
// Windows implementation using SetCursorPos.
func MoveCursor(x, y int) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	setCursorPos := user32.NewProc("SetCursorPos")
	_, _, _ = setCursorPos.Call(uintptr(x), uintptr(y))
}

// PressKey sends a key down followed by a key up for the provided virtual-key code.
// Uses keybd_event on Windows.
func PressKey(vk byte) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	keybdEvent := user32.NewProc("keybd_event")
	const KEYEVENTF_KEYUP = 0x0002
	// key down
	_, _, _ = keybdEvent.Call(uintptr(vk), 0, 0, 0)
	// small sleep to emulate human press duration
	time.Sleep(40 * time.Millisecond)
	// key up
	_, _, _ = keybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
}

// ParseVK converts a key token (e.g. "F3", "R") into a Windows virtual-key code.
// Recognizes F1..F12 and single letters A..Z. Unknown tokens return VK_F3.
func ParseVK(key string) byte {
	k := strings.ToUpper(strings.TrimSpace(key))
	if len(k) == 2 && k[0] == 'F' { // F1-F9
		n := int(k[1] - '0')
		if n >= 1 && n <= 9 {
			return byte(0x70 + (n - 1)) // VK_F1=0x70
		}
	}
	if len(k) == 3 && k[0] == 'F' { // F10-F12
		switch k {
		case "F10":
			return 0x79
		case "F11":
			return 0x7A
		case "F12":
			return 0x7B
		}
	}
	if len(k) == 2 && k[0] == 'F' { // F10-F19 (optional) -> ignore beyond F12 for now
		// fallthrough
	}
	if len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z' {
		return k[0] // 'A'..'Z' match VK codes
	}
	// Default fallback F3
	return 0x72
}

// ListWindows returns titles of top-level visible windows.
// Empty titles are skipped.
func ListWindows() ([]string, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getWindowTextW := user32.NewProc("GetWindowTextW")
	isWindowVisible := user32.NewProc("IsWindowVisible")

	var titles []string
	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		// Skip invisible windows
		vis, _, _ := isWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1 // continue
		}
		// Retrieve window text
		// First get length (approx) by allocating buffer
		const maxChars = 256
		buf := make([]uint16, maxChars)
		r, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if r > 0 {
			// Trim at first zero
			var end int
			for i, v := range buf {
				if v == 0 {
					end = i
					break
				}
			}
			if end == 0 {
				end = int(r)
			}
			s := utf16.Decode(buf[:end])
			title := strings.TrimSpace(string(s))
			if title != "" {
				titles = append(titles, title)
			}
		}
		return 1 // continue enumeration
	})

	// Execute enumeration
	if r, _, callErr := enumWindows.Call(cb, 0); r == 0 {
		if callErr != nil {
			err := callErr
			return nil, err
		}
	}
	return titles, nil
}

// ForegroundWindowTitle returns the title of the current foreground window.
// If no foreground window is available an error is returned.
func ForegroundWindowTitle() (string, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getForegroundWindow := user32.NewProc("GetForegroundWindow")
	getWindowTextW := user32.NewProc("GetWindowTextW")
	hwnd, _, _ := getForegroundWindow.Call()
	if hwnd == 0 {
		err := errors.New("no foreground window")
		return "", err
	}
	const maxChars = 256
	buf := make([]uint16, maxChars)
	r, _, _ := getWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return "", nil
	}
	var end int
	for i, v := range buf {
		if v == 0 {
			end = i
			break
		}
	}
	if end == 0 {
		end = int(r)
	}
	s := utf16.Decode(buf[:end])
	return strings.TrimSpace(string(s)), nil
}
