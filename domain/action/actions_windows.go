package action

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procSendInput        = user32.NewProc("SendInput")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	procGetWindowTxt     = user32.NewProc("GetWindowTextW")
	procIsVisible        = user32.NewProc("IsWindowVisible")
)

const (
	inputMouse    = 0
	inputKeyboard = 1
	keyEventUp    = 0x0002
	mouseRightDn  = 0x0008
	mouseRightUp  = 0x0010
	mouseXDown    = 0x0080
	mouseXUp      = 0x0100
	vkXButton1    = 0x05 // Mouse 4
	vkXButton2    = 0x06 // Mouse 5
)

// xButton returns which mouse side button vk names (1 or 2), or 0 for a key.
func xButton(vk byte) uint32 {
	switch vk {
	case vkXButton1:
		return 1
	case vkXButton2:
		return 2
	}
	return 0
}

// keyboardInput and mouseInput are the two forms of the Win32 INPUT struct. Its
// union starts at offset 8 on 64-bit Windows.
type keyboardInput struct {
	kind      uint32
	_         uint32
	vk, scan  uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
	_         [8]byte // pads to the size of the union's largest member
}

type mouseInput struct {
	kind      uint32
	_         uint32
	dx, dy    int32
	data      uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

func sendInput[T keyboardInput | mouseInput](in T) bool {
	if n, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in)); n != 1 {
		slog.Error("SendInput failed", "error", err)
		return false
	}
	return true
}

// ClickRight sends a right mouse button click (down then up).
func ClickRight() {
	sendInput(mouseInput{kind: inputMouse, flags: mouseRightDn})
	time.Sleep(30 * time.Millisecond)
	sendInput(mouseInput{kind: inputMouse, flags: mouseRightUp})
}

// BeginRightHold presses the right button and returns its release. A nil
// result means the button was already held or Windows rejected the press.
func BeginRightHold() func() {
	const vkRightButton = 0x02
	state, _, _ := procGetAsyncKeyState.Call(vkRightButton)
	if state&0x8000 != 0 || !sendInput(mouseInput{kind: inputMouse, flags: mouseRightDn}) {
		return nil
	}
	return func() {
		if !sendInput(mouseInput{kind: inputMouse, flags: mouseRightUp}) {
			sendInput(mouseInput{kind: inputMouse, flags: mouseRightUp})
		}
	}
}

// MoveCursor moves the OS mouse pointer to (x, y) and reports success.
func MoveCursor(x, y int) bool {
	if r, _, err := procSetCursorPos.Call(uintptr(x), uintptr(y)); r == 0 {
		slog.Error("SetCursorPos failed", "error", err)
		return false
	}
	return true
}

// CursorPos returns the current position of the OS mouse pointer.
func CursorPos() (x, y int, ok bool) {
	var p struct{ X, Y int32 }
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y), r != 0
}

// PressKey sends a key down followed by a key up for the virtual-key code, or
// a click for a mouse side button.
func PressKey(vk byte) {
	if b := xButton(vk); b != 0 {
		sendInput(mouseInput{kind: inputMouse, flags: mouseXDown, data: b})
		time.Sleep(40 * time.Millisecond)
		sendInput(mouseInput{kind: inputMouse, flags: mouseXUp, data: b})
		return
	}
	sendInput(keyboardInput{kind: inputKeyboard, vk: uint16(vk)})
	time.Sleep(40 * time.Millisecond)
	sendInput(keyboardInput{kind: inputKeyboard, vk: uint16(vk), flags: keyEventUp})
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
