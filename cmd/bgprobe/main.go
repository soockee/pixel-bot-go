// Command bgprobe checks whether the game can be captured while it is covered.
//
// Usage: go run ./cmd/bgprobe [-title "World of Warcraft"] [-wait 5s]
//
// Start it, then alt-tab to another window (keep the game open behind it, not
// minimized). After -wait it writes before.png, screen.png and after.png to -out.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/soocke/pixel-bot-go/domain/capture"
	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	procSetDPI       = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetClient    = user32.NewProc("GetClientRect")
	procGetDC        = user32.NewProc("GetDC")
	procReleaseDC    = user32.NewProc("ReleaseDC")
	procPrintWindow  = user32.NewProc("PrintWindow")
	procFindWindowEx = user32.NewProc("FindWindowExW")
	procCreateDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelect       = gdi32.NewProc("SelectObject")
	procGetDIBits    = gdi32.NewProc("GetDIBits")
	procDeleteObject = gdi32.NewProc("DeleteObject")
	procDeleteDC     = gdi32.NewProc("DeleteDC")
)

func main() {
	title := flag.String("title", "World of Warcraft", "game window title")
	wait := flag.Duration("wait", 5*time.Second, "time to alt-tab away before the probe runs")
	settle := flag.Duration("settle", 3*time.Second, "time before the after frame")
	out := flag.String("out", filepath.Join("datasets", "bgprobe"), "output folder")
	watchFor := flag.Duration("watch", 0, "watch an existing bobber like a background run for this long")
	load := flag.Bool("load", false, "with -watch: grab the scene and poll the corner signals like the app")
	pid := flag.Int("pid", 0, "game process id, when several windows share the title")
	flag.Parse()

	procSetDPI.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4): physical pixels
	game, err := capture.ResolveWindow(*title)
	if *pid != 0 {
		game, err = byPID(*title, uint32(*pid))
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	if *watchFor > 0 {
		watch(game, *watchFor, *out, *load)
		return
	}
	fmt.Printf("alt-tab away from %q now; probing in %v\n", *title, *wait)
	time.Sleep(*wait)
	fmt.Println("game in front:", game.Focused())

	save(*out, "before.png", printWindow(game.Handle))
	if area, err := game.Area(); err == nil {
		if f, err := capture.Grab(area); err == nil {
			save(*out, "screen.png", f.Image)
		}
	}
	time.Sleep(*settle)
	save(*out, "after.png", printWindow(game.Handle))
}

// printWindow asks the window to draw its client area into a bitmap, which
// works while other windows cover it. PW_RENDERFULLCONTENT covers DirectX.
func printWindow(hwnd windows.HWND) *image.RGBA {
	var rc struct{ Left, Top, Right, Bottom int32 }
	procGetClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc)))
	w, h := int(rc.Right), int(rc.Bottom)
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bmp, _, _ := procCreateBitmap.Call(screen, uintptr(w), uintptr(h))
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelect.Call(mem, bmp)
	const pwClientOnly, pwRenderFullContent = 1, 2
	if ok, _, err := procPrintWindow.Call(uintptr(hwnd), mem, pwClientOnly|pwRenderFullContent); ok == 0 {
		log.Printf("PrintWindow failed: %v", err)
	}
	info := struct {
		Size, Width, Height    int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		_                      [4]int32
	}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	procSelect.Call(mem, old) // GetDIBits wants the bitmap out of any DC
	procGetDIBits.Call(mem, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&info)), 0)
	for i := 0; i < len(img.Pix); i += 4 { // BGRA to RGBA, opaque
		img.Pix[i], img.Pix[i+2], img.Pix[i+3] = img.Pix[i+2], img.Pix[i], 255
	}
	return img
}

// byPID picks the window titled title that belongs to process pid.
func byPID(title string, pid uint32) (capture.WindowIdentity, error) {
	name, _ := windows.UTF16PtrFromString(title)
	for next := uintptr(0); ; {
		next, _, _ = procFindWindowEx.Call(0, next, 0, uintptr(unsafe.Pointer(name)))
		if next == 0 {
			return capture.WindowIdentity{}, fmt.Errorf("no window titled %q in process %d", title, pid)
		}
		var owner uint32
		windows.GetWindowThreadProcessId(windows.HWND(next), &owner)
		if owner == pid {
			return capture.WindowIdentity{Handle: windows.HWND(next), PID: pid, Title: title}, nil
		}
	}
}

func save(dir, name string, img image.Image) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if !strings.HasPrefix(name, "0") { // watch saves hundreds of numbered frames quietly
		fmt.Println("wrote", f.Name())
	}
}
