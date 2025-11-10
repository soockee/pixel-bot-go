//go:build windows

package capture

// Windows screen capture using per-frame GDI allocations.
// Each Grab/GrabSelection creates a temporary DIB, BitBlt's the screen
// into it, converts BGRA->RGBA into a heap-owned *image.RGBA, and frees
// GDI resources.

import (
	"errors"
	"fmt"
	"image"
	"syscall"
	"unsafe"
)

// Win32 constants
const (
	smCxScreen   = 0
	smCyScreen   = 1
	srccopy      = 0x00CC0020
	dibRGBColors = 0
	biRgb        = 0
)

// Win32 DLL procs (lazy loaded)
var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procGetLastError       = kernel32.NewProc("GetLastError")
)

// BITMAPINFO structures (Win32 layout).
type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	_      [4]byte // one RGBQUAD placeholder (unused for 32-bit)
}

// Per-frame allocation; no persistent globals.

// Grab captures the full screen and returns a newly allocated RGBA image.
func Grab() (*image.RGBA, error) {
	w := int(getSystemMetric(smCxScreen))
	h := int(getSystemMetric(smCyScreen))
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("capture: invalid screen size w=%d h=%d", w, h)
	}
	return captureRect(image.Rect(0, 0, w, h))
}

// GrabSelection captures sel (clipped to screen bounds) and returns an RGBA image.
func GrabSelection(sel image.Rectangle) (*image.RGBA, error) {
	if sel.Empty() {
		return nil, errors.New("capture: empty selection")
	}
	sw := int(getSystemMetric(smCxScreen))
	sh := int(getSystemMetric(smCyScreen))
	screen := image.Rect(0, 0, sw, sh)
	r := sel.Intersect(screen)
	if r.Empty() {
		return nil, fmt.Errorf("capture: selection out of bounds sel=%v screen=%v", sel, screen)
	}
	return captureRect(r)
}

// captureRect performs BitBlt into a top-down DIB section and returns a
// newly allocated *image.RGBA containing the captured pixels.
func captureRect(r image.Rectangle) (*image.RGBA, error) {
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("capture: invalid rect %v", r)
	}

	// Acquire screen DC.
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("capture: GetDC failed winerr=%d", getLastError())
	}
	defer procReleaseDC.Call(0, screenDC)

	// Create compatible memory DC.
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("capture: CreateCompatibleDC failed winerr=%d", getLastError())
	}
	defer procDeleteDC.Call(memDC)

	// Set up BITMAPINFO for top-down 32-bit DIB.
	var bi bitmapInfo
	bi.Header.BiSize = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.BiWidth = int32(w)
	bi.Header.BiHeight = -int32(h) // top-down
	bi.Header.BiPlanes = 1
	bi.Header.BiBitCount = 32
	bi.Header.BiCompression = biRgb
	bi.Header.BiSizeImage = uint32(w * h * 4)

	var bitsPtr unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), dibRGBColors, uintptr(unsafe.Pointer(&bitsPtr)), 0, 0)
	if bmp == 0 {
		return nil, fmt.Errorf("capture: CreateDIBSection failed winerr=%d", getLastError())
	}
	defer procDeleteObject.Call(bmp)

	// Select bitmap into DC.
	prev, _, _ := procSelectObject.Call(memDC, bmp)
	if prev == 0 || prev == ^uintptr(0) { // failure or GDI_ERROR
		return nil, fmt.Errorf("capture: SelectObject failed winerr=%d", getLastError())
	}

	// BitBlt into memory DC at requested offset.
	ok, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(r.Min.X), uintptr(r.Min.Y), srccopy)
	if ok == 0 {
		return nil, fmt.Errorf("capture: BitBlt failed x=%d y=%d w=%d h=%d winerr=%d", r.Min.X, r.Min.Y, w, h, getLastError())
	}

	// Copy & convert BGRA in DIB to RGBA in Go heap slice.
	pixLen := w * h * 4
	src := (*[1 << 30]byte)(bitsPtr)[:pixLen:pixLen]
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// dst.Pix layout already contiguous stride = w*4
	for i := 0; i < pixLen; i += 4 {
		b := src[i+0]
		g := src[i+1]
		r8 := src[i+2]
		// src[i+3] alpha (undefined); force opaque
		dst.Pix[i+0] = r8
		dst.Pix[i+1] = g
		dst.Pix[i+2] = b
		dst.Pix[i+3] = 0xFF
	}
	return dst, nil
}

func getSystemMetric(idx int) int32 {
	v, _, _ := procGetSystemMetrics.Call(uintptr(idx))
	return int32(v)
}

func getLastError() uint32 {
	v, _, _ := procGetLastError.Call()
	return uint32(v)
}
