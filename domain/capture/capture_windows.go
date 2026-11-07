//go:build windows

package capture

// Windows screen capture using per-frame GDI allocations.
// Each Next creates a temporary DIB, BitBlt's the screen into it, converts
// BGRA->RGBA into a heap-owned *image.RGBA, and frees GDI resources.

import (
	"errors"
	"fmt"
	"image"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 constants
const (
	smXVirtual   = 76 // SM_XVIRTUALSCREEN: left of the whole multi-monitor desktop
	smYVirtual   = 77
	smCxVirtual  = 78
	smCyVirtual  = 79
	srccopy      = 0x00CC0020
	dibRGBColors = 0
	biRgb        = 0
)

// Win32 DLL procs (lazy loaded)
var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
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

// Next captures the current region, clipped to the virtual desktop. A region
// on a second monitor can have negative coordinates.
func (s Source) Next() (Frame, error) {
	sel := s.Region()
	if sel == nil || sel.Empty() {
		return Frame{}, errors.New("capture: no region selected")
	}
	vx, vy := int(getSystemMetric(smXVirtual)), int(getSystemMetric(smYVirtual))
	screen := image.Rect(vx, vy, vx+int(getSystemMetric(smCxVirtual)), vy+int(getSystemMetric(smCyVirtual)))
	r := sel.Intersect(screen)
	if r.Empty() {
		return Frame{}, fmt.Errorf("capture: selection out of bounds sel=%v screen=%v", sel, screen)
	}
	img, err := captureRect(r)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Image: img, Origin: r.Min, CapturedAt: time.Now()}, nil
}

// captureRect performs BitBlt into a top-down DIB section and returns a
// newly allocated *image.RGBA containing the captured pixels.
func captureRect(r image.Rectangle) (*image.RGBA, error) {
	w, h := r.Dx(), r.Dy()

	// Acquire screen DC.
	screenDC, _, err := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("capture: GetDC failed: %w", err)
	}
	defer procReleaseDC.Call(0, screenDC)

	// Create compatible memory DC.
	memDC, _, err := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("capture: CreateCompatibleDC failed: %w", err)
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
	bmp, _, err := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), dibRGBColors, uintptr(unsafe.Pointer(&bitsPtr)), 0, 0)
	if bmp == 0 {
		return nil, fmt.Errorf("capture: CreateDIBSection failed: %w", err)
	}
	defer procDeleteObject.Call(bmp)

	// Select bitmap into DC.
	prev, _, err := procSelectObject.Call(memDC, bmp)
	if prev == 0 || prev == ^uintptr(0) { // failure or GDI_ERROR
		return nil, fmt.Errorf("capture: SelectObject failed: %w", err)
	}

	// BitBlt into memory DC at requested offset.
	ok, _, err := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(r.Min.X), uintptr(r.Min.Y), srccopy)
	if ok == 0 {
		return nil, fmt.Errorf("capture: BitBlt failed for %v: %w", r, err)
	}

	// Copy & convert BGRA in DIB to RGBA in Go heap slice.
	pixLen := w * h * 4
	src := unsafe.Slice((*byte)(bitsPtr), pixLen)
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
