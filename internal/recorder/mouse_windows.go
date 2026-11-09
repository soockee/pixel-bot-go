package recorder

import (
	"context"
	"errors"
	"fmt"
	"image"
	"runtime"
	"time"
	"unsafe"

	"github.com/soocke/pixel-bot-go/internal/playtrace"

	"golang.org/x/sys/windows"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	getForegroundWindow      = user32.NewProc("GetForegroundWindow")
	setThreadDPI             = user32.NewProc("SetThreadDpiAwarenessContext")
	screenToClient           = user32.NewProc("ScreenToClient")
	clientToScreen           = user32.NewProc("ClientToScreen")
	getClientRect            = user32.NewProc("GetClientRect")
	windowFromPoint          = user32.NewProc("WindowFromPoint")
	getAncestor              = user32.NewProc("GetAncestor")
	setWindowsHook           = user32.NewProc("SetWindowsHookExW")
	callNextHook             = user32.NewProc("CallNextHookEx")
	unhookWindowsHook        = user32.NewProc("UnhookWindowsHookEx")
	setTimer                 = user32.NewProc("SetTimer")
	killTimer                = user32.NewProc("KillTimer")
	getMessage               = user32.NewProc("GetMessageW")
	getModuleHandle          = kernel32.NewProc("GetModuleHandleW")
	createWindow             = user32.NewProc("CreateWindowExW")
	destroyWindow            = user32.NewProc("DestroyWindow")
	registerRawInput         = user32.NewProc("RegisterRawInputDevices")
	getRawInput              = user32.NewProc("GetRawInputData")
	defWindowProc            = user32.NewProc("DefWindowProcW")
	getCursorPos             = user32.NewProc("GetCursorPos")
	getTickCount             = kernel32.NewProc("GetTickCount")
)

const (
	gaRoot  = 2
	wmTimer = 0x0113
	wmInput = 0x00FF
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}

type windowTarget struct {
	handle uintptr
	pid    uint32
}

type inputEvent = playtrace.Event
type clientPosition = playtrace.Position

type segmentTracker struct {
	number int
	active bool
}

func (s *segmentTracker) change(eligible bool, elapsed int64, emit func(inputEvent)) {
	if eligible == s.active {
		return
	}
	s.active = eligible
	kind := "segment_end"
	if eligible {
		s.number++
		kind = "segment_start"
	}
	emit(inputEvent{Type: kind, ElapsedUS: elapsed, Segment: s.number})
}

func (w windowTarget) stillExists() bool {
	var pid uint32
	thread, _, _ := getWindowThreadProcessID.Call(w.handle, uintptr(unsafe.Pointer(&pid)))
	return thread != 0 && pid == w.pid
}

func (w windowTarget) focused() bool {
	handle, _, _ := getForegroundWindow.Call()
	return handle == w.handle
}

// clientOrigin returns the screen position of the client area, in the same DPI
// context as the positions in the trace.
func (w windowTarget) clientOrigin() (image.Point, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previousDPI, _, err := setThreadDPI.Call(^uintptr(3))
	if previousDPI == 0 {
		return image.Point{}, fmt.Errorf("set DPI context: %w", err)
	}
	defer setThreadDPI.Call(previousDPI)
	var origin point
	if ok, _, err := clientToScreen.Call(w.handle, uintptr(unsafe.Pointer(&origin))); ok == 0 {
		return image.Point{}, fmt.Errorf("find client area: %w", err)
	}
	if isMinimized(origin) {
		return image.Point{}, errors.New("the selected window is minimized")
	}
	return image.Pt(int(origin.X), int(origin.Y)), nil
}

// isMinimized recognizes where Windows parks minimized windows.
func isMinimized(origin point) bool { return origin.X <= -32000 && origin.Y <= -32000 }

func (w windowTarget) position(screen point) (clientPosition, bool) {
	// POINT is passed by value. Windows packs its two LONGs into one
	// argument on 64-bit targets and into two stack slots on 32-bit targets.
	var underCursor uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		packed := uint64(uint32(screen.X)) | uint64(uint32(screen.Y))<<32
		underCursor, _, _ = windowFromPoint.Call(uintptr(packed))
	} else {
		underCursor, _, _ = windowFromPoint.Call(uintptr(screen.X), uintptr(screen.Y))
	}
	root, _, _ := getAncestor.Call(underCursor, gaRoot)
	if root != w.handle {
		return clientPosition{}, false
	}
	client := screen
	converted, _, _ := screenToClient.Call(w.handle, uintptr(unsafe.Pointer(&client)))
	var bounds rect
	gotBounds, _, _ := getClientRect.Call(w.handle, uintptr(unsafe.Pointer(&bounds)))
	if converted == 0 || gotBounds == 0 || client.X < 0 || client.Y < 0 ||
		client.X >= bounds.Right || client.Y >= bounds.Bottom {
		return clientPosition{}, false
	}
	return clientPosition{X: client.X, Y: client.Y, Width: bounds.Right, Height: bounds.Bottom}, true
}

type rawInputDevice struct {
	UsagePage, Usage uint16
	Flags            uint32
	Target           uintptr
}

type rawInputHeader struct {
	Type, Size     uint32
	Device, WParam uintptr
}

type rawMouse struct {
	Flags                   uint16
	_                       uint16
	ButtonFlags, ButtonData uint16
	RawButtons              uint32
	DX, DY                  int32
	ExtraInformation        uint32
}

type rawInputPacket struct {
	Header rawInputHeader
	Mouse  rawMouse
}

func openMouseReceiver() (uintptr, error) {
	class, err := windows.UTF16PtrFromString("STATIC")
	if err != nil {
		return 0, err
	}
	// HWND_MESSAGE creates a hidden message-only window. INPUTSINK lets
	// the recorder receive hardware packets while its target has focus.
	handle, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if handle == 0 {
		return 0, fmt.Errorf("create raw mouse receiver: %w", err)
	}
	device := rawInputDevice{UsagePage: 1, Usage: 2, Flags: 0x100, Target: handle}
	ok, _, err := registerRawInput.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	if ok == 0 {
		destroyWindow.Call(handle)
		return 0, fmt.Errorf("register raw mouse: %w", err)
	}
	return handle, nil
}

func closeMouseReceiver(handle uintptr) {
	device := rawInputDevice{UsagePage: 1, Usage: 2, Flags: 1} // RIDEV_REMOVE requires a null target.
	registerRawInput.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	destroyWindow.Call(handle)
}

func readMousePacket(handle uintptr) (rawMouse, error) {
	var packet rawInputPacket
	size := uint32(unsafe.Sizeof(packet))
	count, _, err := getRawInput.Call(handle, 0x10000003, uintptr(unsafe.Pointer(&packet)),
		uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(packet.Header))
	if uint32(count) == ^uint32(0) {
		return rawMouse{}, fmt.Errorf("read raw mouse packet: %w", err)
	}
	if count < unsafe.Sizeof(packet) || packet.Header.Type != 0 {
		return rawMouse{}, errors.New("unexpected raw mouse packet")
	}
	return packet.Mouse, nil
}

func capture(ctx context.Context, target windowTarget, start time.Time, out chan<- inputEvent, ready func(), recordKeys bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	previousDPI, _, err := setThreadDPI.Call(^uintptr(3))
	if previousDPI == 0 {
		return fmt.Errorf("set recorder DPI awareness: %w", err)
	}
	defer setThreadDPI.Call(previousDPI)

	timer, _, err := setTimer.Call(0, 0, 50, 0)
	if timer == 0 {
		return fmt.Errorf("start recorder message timer: %w", err)
	}
	defer killTimer.Call(0, timer)

	receiver, err := openMouseReceiver()
	if err != nil {
		return err
	}
	defer closeMouseReceiver(receiver)

	var scope segmentTracker
	var mouse gestureTracker
	var keys keyboardTracker
	overflow := false
	focused := false
	var focusedSince uint32
	emit := func(event inputEvent) {
		select {
		case out <- event:
		default:
			overflow = true
		}
	}
	nowUS := func() int64 { return time.Since(start).Microseconds() }
	endScope := func(now int64) {
		focused = false
		mouse.leave(now, scope.number, emit)
		keys.clear()
		scope.change(false, now, emit)
	}
	observeFocus := func(now int64) bool {
		if !target.focused() {
			endScope(now)
			return false
		}
		if !focused {
			tick, _, _ := getTickCount.Call()
			focusedSince = uint32(tick)
			focused = true
		}
		return true
	}
	if recordKeys {
		unhook, err := openKeyboardHook(func(d keyboardData, down bool) {
			if ctx.Err() != nil || overflow {
				return
			}
			now := nowUS()
			if !observeFocus(now) {
				return
			}
			scope.change(true, now, emit)
			emit(keys.event(d, down, now, scope.number))
		})
		if err != nil {
			return err
		}
		defer unhook()
	}
	ready()

	var msg message
	for ctx.Err() == nil && !overflow {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return fmt.Errorf("read recorder messages: %w", err)
		case 0:
			return errors.New("recorder message loop stopped unexpectedly")
		}
		switch {
		case msg.Message == wmTimer:
			if !target.stillExists() {
				if !overflow {
					endScope(nowUS())
				}
				if overflow {
					return errors.New("recording buffer filled; trace is incomplete")
				}
				return nil
			}
			observeFocus(nowUS())
		case msg.Message == wmInput && msg.Window == receiver:
			err := func() error {
				defer defWindowProc.Call(msg.Window, uintptr(msg.Message), msg.WParam, msg.LParam)
				if ctx.Err() != nil || overflow {
					return nil
				}
				now := nowUS()
				// WM_INPUT is queued. Discard packets stamped before the
				// recorder confirmed the target had foreground focus.
				if !observeFocus(now) || int32(msg.Time-focusedSince) <= 0 {
					return nil
				}
				var screen point
				ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&screen)))
				if ok == 0 {
					mouse.leave(now, scope.number, emit)
					return nil
				}
				position, eligible := target.position(screen)
				if !eligible {
					mouse.leave(now, scope.number, emit)
					return nil
				}
				packet, err := readMousePacket(msg.LParam)
				if err != nil {
					return err
				}
				scope.change(true, now, emit)
				mouse.packet(packet, position, now, scope.number, emit)
				return nil
			}()
			if err != nil {
				return err
			}
		}
	}
	if !overflow {
		endScope(nowUS())
	}
	if overflow {
		return errors.New("recording buffer filled; trace is incomplete")
	}
	return nil
}
