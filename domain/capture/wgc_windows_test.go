package capture

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procRegisterClass  = user32.NewProc("RegisterClassExW")
	procCreateWindow   = user32.NewProc("CreateWindowExW")
	procDestroyWindow  = user32.NewProc("DestroyWindow")
	procDefWindowProc  = user32.NewProc("DefWindowProcW")
	procPeekMessage    = user32.NewProc("PeekMessageW")
	procDispatchMsg    = user32.NewProc("DispatchMessageW")
	procBeginPaint     = user32.NewProc("BeginPaint")
	procEndPaint       = user32.NewProc("EndPaint")
	procFillRect       = user32.NewProc("FillRect")
	procCreateBrush    = gdi32.NewProc("CreateSolidBrush")
	procSetForeground  = user32.NewProc("SetForegroundWindow")
	testSquare         = image.Rect(40, 30, 80, 60) // client pixels painted blue
	testWindowTitle, _ = windows.UTF16PtrFromString("Go Fish capture test")
)

// paintTestWindow paints the client red with a blue testSquare.
func paintTestWindow(hwnd, msg, wparam, lparam uintptr) uintptr {
	const wmPaint = 0x000F
	if msg != wmPaint {
		r, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
		return r
	}
	var ps [72]byte // PAINTSTRUCT
	dc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	red, _, _ := procCreateBrush.Call(0x0000FF) // COLORREF is 0x00BBGGRR
	blue, _, _ := procCreateBrush.Call(0xFF0000)
	all := [4]int32{0, 0, 4000, 4000}
	square := [4]int32{int32(testSquare.Min.X), int32(testSquare.Min.Y), int32(testSquare.Max.X), int32(testSquare.Max.Y)}
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&all)), red)
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&square)), blue)
	procDeleteObject.Call(red)
	procDeleteObject.Call(blue)
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	return 0
}

// openTestWindow shows a window with a title bar, so its client area sits
// inside the frame that Windows Graphics Capture records, and pumps its
// messages until the test ends.
func openTestWindow(t *testing.T) WindowIdentity {
	t.Helper()
	ready := make(chan windows.HWND)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer physicalPixels()()
		class := struct {
			Size, Style                uint32
			WndProc                    uintptr
			ClassExtra, WindowExtra    int32
			Instance, Icon, Cursor, Bg uintptr
			MenuName, ClassName        *uint16
			IconSm                     uintptr
		}{WndProc: windows.NewCallback(paintTestWindow), ClassName: testWindowTitle}
		class.Size = uint32(unsafe.Sizeof(class))
		procRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
		const overlappedWindow, visible = 0x00CF0000, 0x10000000
		hwnd, _, _ := procCreateWindow.Call(0, uintptr(unsafe.Pointer(testWindowTitle)), uintptr(unsafe.Pointer(testWindowTitle)),
			overlappedWindow|visible, 200, 200, 400, 300, 0, 0, 0, 0)
		procSetForeground.Call(hwnd)
		ready <- windows.HWND(hwnd)
		var msg [48]byte
		for {
			select {
			case <-done:
				procDestroyWindow.Call(hwnd)
				return
			default:
			}
			for {
				const remove = 1
				if got, _, _ := procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, remove); got == 0 {
					break
				}
				procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	hwnd := <-ready
	if hwnd == 0 {
		t.Fatal("could not create the test window")
	}
	w := WindowIdentity{Handle: hwnd, Title: "Go Fish capture test"}
	windows.GetWindowThreadProcessId(hwnd, &w.PID)
	return w
}

func TestWindowStreamGrabsClientCoordinates(t *testing.T) {
	w := openTestWindow(t)
	s, err := OpenWindowStream(w)
	if err != nil {
		t.Skip("Windows Graphics Capture unavailable:", err)
	}
	defer s.Close()
	client, err := w.Area()
	if err != nil {
		t.Fatal(err)
	}
	// The blue square and a margin of red around it, in screen coordinates.
	area := testSquare.Inset(-10).Add(client.Min)
	var f Frame
	for deadline := time.Now().Add(2 * time.Second); ; {
		f, err = s.Grab(area)
		if err == nil && f.Image.RGBAAt(20, 20) == (color.RGBA{0, 0, 255, 255}) || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond) // the first frames may predate the first paint
	}
	if err != nil {
		t.Fatal(err)
	}
	if f.Origin != area.Min || f.Image.Bounds().Size() != area.Size() {
		t.Fatalf("frame %v at %v, want %v at %v", f.Image.Bounds().Size(), f.Origin, area.Size(), area.Min)
	}
	blue, red := color.RGBA{0, 0, 255, 255}, color.RGBA{255, 0, 0, 255}
	for _, c := range []struct {
		p    image.Point
		want color.RGBA
	}{{image.Pt(10, 10), blue}, {image.Pt(49, 39), blue}, {image.Pt(9, 9), red}, {image.Pt(50, 40), red}, {image.Pt(30, 2), red}} {
		if got := f.Image.RGBAAt(c.p.X, c.p.Y); got != c.want {
			t.Errorf("pixel %v = %v, want %v", c.p, got, c.want)
		}
	}
}
