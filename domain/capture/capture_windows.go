//go:build windows

package capture

// Windows screen capture using a single persistent DIB section backing buffer.
// This eliminates per-frame Go heap allocations: the same *image.RGBA instance
// is mutated in place for every capture. Consumers that need to retain pixel
// data across frames must copy it themselves. The implementation recreates
// GDI resources only when the requested capture rectangle dimensions change.

import (
	"errors"
	"fmt"
	"image"
	"sync"
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

// Minimal type aliases for clarity.
type (
	handle  uintptr
	hdc     handle
	hbitmap handle
)

// BITMAPINFO structures (matching Win32 layout).
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

// persistent resources guarded by a mutex.
var captureState struct {
	mu      sync.Mutex
	w, h    int
	memDC   hdc
	bmp     hbitmap
	bitsPtr unsafe.Pointer
	img     *image.RGBA // reused frame (BGRA converted to RGBA each capture)
}

// Grab returns a full-screen frame using the persistent buffer.
func Grab() (*image.RGBA, error) {
	w := int(getSystemMetric(smCxScreen))
	h := int(getSystemMetric(smCyScreen))
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("capture: invalid screen size w=%d h=%d", w, h)
	}
	return captureRect(image.Rect(0, 0, w, h))
}

// GrabSelection captures the provided rectangle (clipped to screen bounds).
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

// captureRect performs the BitBlt into a persistent top-down DIB section and
// returns the single reused RGBA image. Pixels are updated in place.
func captureRect(r image.Rectangle) (*image.RGBA, error) {
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("capture: invalid rect %v", r)
	}

	captureState.mu.Lock()
	defer captureState.mu.Unlock()

	// (Re)initialize resources if size changed or first use.
	if captureState.bmp == 0 || w != captureState.w || h != captureState.h {
		releaseResourcesLocked()
		if err := allocateResourcesLocked(w, h); err != nil {
			releaseResourcesLocked()
			return nil, err
		}
	}

	// Acquire screen DC for source each frame (cheap, avoids leaking if caller stops).
	srcDC, _, _ := procGetDC.Call(0)
	if srcDC == 0 {
		return nil, fmt.Errorf("capture: GetDC failed winerr=%d", getLastError())
	}
	defer procReleaseDC.Call(0, srcDC)

	// Select bitmap into memDC (already selected after allocation, so skip unless changed).
	// Perform BitBlt at requested offset.
	ok, _, _ := procBitBlt.Call(uintptr(captureState.memDC), 0, 0, uintptr(w), uintptr(h), srcDC, uintptr(r.Min.X), uintptr(r.Min.Y), srccopy)
	if ok == 0 {
		return nil, fmt.Errorf("capture: BitBlt failed x=%d y=%d w=%d h=%d winerr=%d", r.Min.X, r.Min.Y, w, h, getLastError())
	}

	// Map DIB memory into slice (no allocation; slice header updated only when resized).
	pixLen := w * h * 4
	header := (*[1 << 30]byte)(captureState.bitsPtr)[:pixLen:pixLen] // limits capacity to pixLen
	if captureState.img == nil || cap(captureState.img.Pix) < pixLen {
		captureState.img = &image.RGBA{Pix: make([]byte, pixLen), Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	} else {
		captureState.img.Pix = captureState.img.Pix[:pixLen]
		captureState.img.Stride = w * 4
		captureState.img.Rect = image.Rect(0, 0, w, h)
	}

	// Copy & convert BGRA -> RGBA (alpha forced to 0xFF). NOTE: Could remove copy and swap
	// in place if we exposed the raw DIB memory directly, but we keep a separate Go-managed
	// slice to avoid accidental use-after-free should we recreate the DIB.
	dst := captureState.img.Pix
	for i := 0; i < pixLen; i += 4 {
		b := header[i]
		g := header[i+1]
		r8 := header[i+2]
		dst[i] = r8
		dst[i+1] = g
		dst[i+2] = b
		dst[i+3] = 0xFF
	}
	return captureState.img, nil
}

// allocateResourcesLocked creates a memory DC and a top-down DIB section sized w*h.
func allocateResourcesLocked(w, h int) error {
	// Create a compatible memory DC using the desktop DC as template.
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return fmt.Errorf("capture: GetDC failed winerr=%d", getLastError())
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return fmt.Errorf("capture: CreateCompatibleDC failed winerr=%d", getLastError())
	}

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
		procDeleteDC.Call(memDC)
		return fmt.Errorf("capture: CreateDIBSection failed winerr=%d", getLastError())
	}

	// Select bitmap into DC.
	prev, _, _ := procSelectObject.Call(memDC, bmp)
	if prev == 0 || prev == ^uintptr(0) { // failure or GDI_ERROR
		procDeleteObject.Call(bmp)
		procDeleteDC.Call(memDC)
		return fmt.Errorf("capture: SelectObject failed winerr=%d", getLastError())
	}
	// We don't need the previous object handle; leave it selected out.

	captureState.memDC = hdc(memDC)
	captureState.bmp = hbitmap(bmp)
	captureState.bitsPtr = bitsPtr
	captureState.w = w
	captureState.h = h
	return nil
}

// releaseResourcesLocked frees GDI objects; caller must hold captureState.mu.
func releaseResourcesLocked() {
	if captureState.bmp != 0 {
		procDeleteObject.Call(uintptr(captureState.bmp))
	}
	if captureState.memDC != 0 {
		procDeleteDC.Call(uintptr(captureState.memDC))
	}
	captureState.bmp = 0
	captureState.memDC = 0
	captureState.bitsPtr = nil
	// captureState.img retained; will be resized or reused.
}

func getSystemMetric(idx int) int32 {
	v, _, _ := procGetSystemMetrics.Call(uintptr(idx))
	return int32(v)
}

func getLastError() uint32 {
	v, _, _ := procGetLastError.Call()
	return uint32(v)
}
