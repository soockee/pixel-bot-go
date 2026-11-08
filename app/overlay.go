package app

import (
	"image"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"github.com/soocke/pixel-bot-go/domain/capture"
)

// offScreen parks the overlay while no run shows on it.
var offScreen = application.Rect{X: -32000, Y: -32000, Width: 1, Height: 1}

// newOverlay builds a see-through window that lies over the game's client area
// during a run and shows how it goes. It never takes focus or clicks (see
// passThrough), so the game stays in front and the run keeps going, and it
// stays off the taskbar and Alt+Tab. It is shown once at launch, while the app
// itself is in front, and only moved after that, because showing a window
// activates it.
func (a *app) newOverlay() *application.WebviewWindow {
	return a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "overlay", Title: "Go Fish overlay", URL: "/overlay.html",
		X: offScreen.X, Y: offScreen.Y, Width: offScreen.Width, Height: offScreen.Height,
		Frameless: true, AlwaysOnTop: true, DisableResize: true,
		BackgroundType: application.BackgroundTypeTransparent,
		Windows:        application.WindowsWindow{HiddenOnTaskbar: true, DisableFramelessWindowDecorations: true},
	})
}

// passThrough makes the overlay ignore focus and the mouse: WS_EX_NOACTIVATE
// keeps it from becoming the foreground window, and WS_EX_TRANSPARENT on a
// layered window at full opacity passes clicks to the game. Wails' own
// IgnoreMouseEvents and ExStyle options lose the transparent background, so the
// styles go on the native window, which exists only some time after the app
// started. It also hides the overlay from screen copies (BitBlt in
// domain/capture), so the bot never sees its own brackets and status. It
// reports whether the click-through styles were set. Call on the main thread.
func (a *app) passThrough() bool {
	hwnd := w32.HWND(a.overlay.NativeWindow())
	if hwnd == 0 {
		return false
	}
	if !w32.SetWindowDisplayAffinity(uintptr(hwnd), w32.WDA_EXCLUDEFROMCAPTURE) {
		a.logger.Warn("overlay is visible to screen capture; the bot may read it")
	}
	style := uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE))
	w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, style|w32.WS_EX_NOACTIVATE|w32.WS_EX_TRANSPARENT|w32.WS_EX_LAYERED)
	return w32.SetLayeredWindowAttributes(hwnd, 0, 255, w32.LWA_ALPHA)
}

// followGame keeps the overlay on the game's client area while a run goes and
// parks it off screen otherwise. It runs on the loop.
func (a *app) followGame() {
	var area image.Rectangle
	if a.container.Capture.Enabled() {
		area, _ = capture.WindowArea(a.container.Capture.Window()) // a minimised or closed game parks it
	}
	if area == a.overlayArea {
		return
	}
	a.overlayArea = area
	bounds := offScreen
	if !area.Empty() {
		bounds = application.Rect{X: area.Min.X, Y: area.Min.Y, Width: area.Dx(), Height: area.Dy()}
	}
	// Async: the main thread may be waiting on the loop during shutdown.
	application.InvokeAsync(func() {
		if !a.overlayReady {
			a.overlayReady = a.passThrough()
		}
		a.overlay.SetPhysicalBounds(bounds)
	})
}
