package app

import (
	"errors"
	"image"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/capture"
)

// offScreen parks the overlay while no run shows on it.
var offScreen = application.Rect{X: -32000, Y: -32000, Width: 1, Height: 1}

// overlayPlacement is what the overlay window was last set to: it covers area
// during a run, or is only the pill, over the same area, while it is moved.
type overlayPlacement struct {
	area image.Rectangle
	move bool
}

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

// setClickThrough makes the overlay pass the mouse on to the game, or take it
// so the pill can be dragged. It keeps the other styles passThrough set. Call on
// the main thread, after passThrough.
func (a *app) setClickThrough(on bool) {
	hwnd := w32.HWND(a.overlay.NativeWindow())
	if hwnd == 0 {
		return
	}
	style := uint32(w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE))
	if on {
		style |= w32.WS_EX_TRANSPARENT
	} else {
		style &^= w32.WS_EX_TRANSPARENT
	}
	w32.SetWindowLong(hwnd, w32.GWL_EXSTYLE, style)
}

// followGame keeps the overlay on the game's client area while a run goes and
// parks it off screen otherwise. While the overlay is moved it is only the pill,
// placed where it was saved. It runs on the loop.
func (a *app) followGame() {
	c := a.container
	var area image.Rectangle
	if c.Capture.Enabled() {
		area, _ = c.frames().Window()
	} else if a.overlayMove {
		area, _ = capture.WindowArea(c.Capture.Window())
	}
	want := overlayPlacement{area: area, move: a.overlayMove}
	if want == a.overlayPlaced {
		if want.move && !area.Empty() {
			application.InvokeAsync(a.trackPill) // the person is dragging it
		}
		return
	}
	a.overlayPlaced = want
	x, y := c.Config.OverlayX, c.Config.OverlayY
	// Async: the main thread may be waiting on the loop during shutdown.
	application.InvokeAsync(func() {
		if !a.overlayReady {
			a.overlayReady = a.passThrough()
		}
		bounds := offScreen
		switch {
		case area.Empty():
		case want.move:
			bounds = a.pillBounds(area, x, y)
		default:
			bounds = application.Rect{X: area.Min.X, Y: area.Min.Y, Width: area.Dx(), Height: area.Dy()}
		}
		a.setClickThrough(!want.move)
		a.overlay.SetPhysicalBounds(bounds)
	})
}

// pillBounds is the overlay window while it is moved: the pill at its saved
// place, at the monitor's scale. It remembers them as the pill's place until the
// person drags it. Call on the main thread.
func (a *app) pillBounds(area image.Rectangle, x, y float64) application.Rect {
	scale := 1.0
	if hwnd := w32.HWND(a.overlay.NativeWindow()); hwnd != 0 {
		if dpi := w32.GetDpiForWindow(hwnd); dpi > 0 {
			scale = float64(dpi) / 96
		}
	}
	r := pillRect(area, x, y, int(pillW*scale), int(pillH*scale))
	a.pillMu.Lock()
	a.pill = r
	a.pillMu.Unlock()
	return application.Rect{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

// trackPill remembers where the person has dragged the pill to. Call on the
// main thread.
func (a *app) trackPill() {
	b := a.overlay.PhysicalBounds()
	a.pillMu.Lock()
	a.pill = image.Rect(b.X, b.Y, b.X+b.Width, b.Y+b.Height)
	a.pillMu.Unlock()
}

// setOverlayMove starts or ends placing the overlay. Ending keeps the place.
// It runs on the loop.
func (a *app) setOverlayMove(on bool) error {
	c := a.container
	if on && !a.overlayMove {
		if area, err := capture.WindowArea(c.Capture.Window()); err != nil || area.Empty() {
			err := errors.New("start the game first, so the overlay has a window to sit on")
			c.View.SetStatus("Overlay: " + err.Error())
			return err
		}
	}
	if !on && a.overlayMove {
		a.saveOverlayPlace()
	}
	a.overlayMove = on
	c.View.SetOverlay(c.Config.OverlayX, c.Config.OverlayY, on)
	return nil
}

// saveOverlayPlace keeps where the pill was left, as a place on the game window.
func (a *app) saveOverlayPlace() {
	c := a.container
	a.pillMu.Lock()
	pill := a.pill
	a.pillMu.Unlock()
	x, y, ok := pillPlace(pill, a.overlayPlaced.area)
	if !ok {
		return
	}
	c.Config.OverlayX, c.Config.OverlayY = x, y
	if err := c.Config.Save(config.FilePath); err != nil {
		a.logger.Error("overlay place not saved", "error", err)
		c.View.SetStatus("Overlay place not saved: " + err.Error())
		return
	}
	a.logger.Info("overlay place saved", "x", x, "y", y)
	c.View.SetStatus("Overlay place saved")
}
