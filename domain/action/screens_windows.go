package action

import (
	"errors"
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type winRect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

const monitorInfoPrimary = 1

// ListScreens returns all monitors, numbered with the primary as 1.
func ListScreens() ([]Screen, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	enum := user32.NewProc("EnumDisplayMonitors")
	getInfo := user32.NewProc("GetMonitorInfoW")

	var rects []image.Rectangle
	primary := 0
	cb := syscall.NewCallback(func(hmon, _, _, _ uintptr) uintptr {
		mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r, _, _ := getInfo.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			if mi.Flags&monitorInfoPrimary != 0 {
				primary = len(rects)
			}
			rects = append(rects, image.Rect(int(mi.Monitor.Left), int(mi.Monitor.Top), int(mi.Monitor.Right), int(mi.Monitor.Bottom)))
		}
		return 1
	})
	if r, _, err := enum.Call(0, 0, cb, 0); r == 0 {
		return nil, err
	}
	if len(rects) == 0 {
		return nil, errors.New("no monitors found")
	}
	return numberScreens(rects, primary), nil
}

// ScreenBounds returns the bounds of monitor n (1-based). It reports false
// when n is 0 (no preference), the monitor does not exist or enumeration fails.
func ScreenBounds(n int) (image.Rectangle, bool) {
	if n < 1 {
		return image.Rectangle{}, false
	}
	screens, err := ListScreens()
	if err != nil {
		return image.Rectangle{}, false
	}
	s, ok := ScreenByNumber(screens, n)
	return s.Bounds, ok
}
