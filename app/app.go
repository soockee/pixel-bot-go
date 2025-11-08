package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

	. "modernc.org/tk9.0"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/capture"
	"github.com/soocke/pixel-bot-go/config"
)

const (
	tick = 100 * time.Millisecond
)

type app struct {
	config  *config.Config
	width   int
	height  int
	ticks   []time.Duration
	start   time.Time
	afterID string
	text    *TextWidget // displays elapsed milliseconds

	// Debug widget for last target detection status
	debugLabel *LabelWidget

	// Capture / detection state
	captureEnabled atomic.Bool
	frameCh        chan *image.RGBA
	targetImg      image.Image
	targetLabel    *LabelWidget // shows the loaded target image
	lastDetectX    int
	lastDetectY    int
	lastDetectOK   atomic.Bool

	captureFrame *LabelWidget
}

func NewApp(title string, width, height int, config *config.Config) *app {
	a := &app{}

	App.WmTitle("")
	a.width = width
	a.height = height

	WmProtocol(App, "WM_DELETE_WINDOW", a.exitHandler)
	WmGeometry(App, fmt.Sprintf("%dx%d+100+100", width, height))
	return a
}

func (a *app) Start() {
	// Create a Text widget for displaying elapsed time.
	a.text = Text(Height(1), Width(30))
	Pack(a.text)

	// Debug label (initial state: no target yet)
	a.debugLabel = Label(Txt("Target: <none>"), Borderwidth(1), Relief("ridge"))
	Pack(a.debugLabel, Padx("1m"), Pady("1m"))

	// Exit button.
	Pack(Button(Txt("Exit"), Command(a.exitHandler)))

	// Toggle capture button.
	Pack(Button(Txt("Toggle Capture"), Command(func() { a.toggleCapture() })))

	// Load template image (target) from embedded bytes and display a small preview.
	if img, err := assets.FishingTargetImage(); err != nil {
		if a.text != nil {
			a.text.Delete("1.0", END)
			a.text.Insert("1.0", fmt.Sprintf("Target load error: %v", err))
		}
	} else {
		a.targetImg = img
		if a.targetLabel == nil {
			a.targetLabel = Label(Image(NewPhoto(Data(assets.FishingTargetPNG))), Borderwidth(1), Relief("groove"))
			Pack(a.targetLabel, Padx("1m"), Pady("1m"))
		} else {
			// If already exists, refresh in case of future template reload.
			a.targetLabel.Configure(Image(NewPhoto(Data(assets.FishingTargetPNG))))
		}
	}

	// Initialize timer start.
	a.start = time.Now()

	// Kick off update loop.
	a.scheduleUpdate()

	App.Wait()
}

func (a *app) update() {
	// Compute elapsed time in milliseconds.
	elapsed := time.Since(a.start).Milliseconds()

	if a.text != nil {
		func() {
			defer func() { _ = recover() }()
			a.text.Delete("1.0", END)
			a.text.Insert("1.0", fmt.Sprintf("Elapsed: %d ms", elapsed))
		}()
	}

	// Non-blocking receive of latest frame and attempt detection.
	if a.captureEnabled.Load() && a.targetImg != nil && a.frameCh != nil {
		select {
		case frame := <-a.frameCh:
			x, y, ok := capture.DetectTemplate(frame, a.targetImg)
			if ok {
				a.lastDetectX, a.lastDetectY = x, y
				a.lastDetectOK.Store(true)
				// Move mouse to detected position.
				moveCursor(x, y)
			} else {
				a.lastDetectOK.Store(false)
			}

			// Update canvas preview (throttled).
			a.updateCaptureFrame(frame)
		default:
		}
	}

	// Append detection info.
	if a.text != nil {
		if a.lastDetectOK.Load() {
			a.text.Insert("end", fmt.Sprintf("\nDetected at (%d,%d)", a.lastDetectX, a.lastDetectY))
			// Update debug widget with latest coordinates
			if a.debugLabel != nil {
				// guard against panic if widget destroyed
				func() {
					defer func() { _ = recover() }()
					a.debugLabel.Configure(Txt(fmt.Sprintf("Target: (%d,%d)", a.lastDetectX, a.lastDetectY)))
				}()
			}
		} else if a.captureEnabled.Load() {
			a.text.Insert("end", "\nNo detection")
			if a.debugLabel != nil {
				func() { defer func() { _ = recover() }(); a.debugLabel.Configure(Txt("Target: <searching>")) }()
			}
		}
	}

	a.scheduleUpdate()
}

func (a *app) exitHandler() {
	// Cancel scheduled after event if any.
	if a.afterID != "" {
		TclAfterCancel(a.afterID)
	}
	Destroy(App)
}

func (a *app) scheduleUpdate() {
	// Schedule the next update using TclAfter to stay on Tk's event loop thread.
	a.afterID = TclAfter(tick, func() { a.update() })
}

// toggleCapture enables or disables the capture goroutine.
func (a *app) toggleCapture() {
	if a.captureEnabled.Load() {
		a.captureEnabled.Store(false)
		// Drain channel to free memory.
		if a.frameCh != nil {
			select {
			case <-a.frameCh:
			default:
			}
		}
		if a.captureFrame != nil {
			func() { defer func() { _ = recover() }(); Destroy(a.captureFrame) }()
			a.captureFrame = nil
		}
		// Reset debug widget state
		if a.debugLabel != nil {
			func() { defer func() { _ = recover() }(); a.debugLabel.Configure(Txt("Target: <none>")) }()
		}
		// Clear last detection status
		a.lastDetectOK.Store(false)
		if a.text != nil {
			a.text.Insert("end", "\nCapture OFF")
		}
		return
	}
	// Enable capture
	if a.frameCh == nil {
		a.frameCh = make(chan *image.RGBA, 1)
	}
	a.captureEnabled.Store(true)

	go a.captureLoop()
	if a.text != nil {
		a.text.Insert("end", "\nCapture ON")
	}
}

// captureLoop runs in a goroutine and pushes latest frames.
func (a *app) captureLoop() {
	for a.captureEnabled.Load() {
		img, err := capture.Grab()
		if err == nil && img != nil {
			select {
			case a.frameCh <- img:
			default: // drop if busy to avoid blocking
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// moveCursor moves the OS mouse pointer (Windows only).
func moveCursor(x, y int) {
	// Windows SetCursorPos
	user32 := windows.NewLazySystemDLL("user32.dll")
	setCursorPos := user32.NewProc("SetCursorPos")
	_, _, _ = setCursorPos.Call(uintptr(x), uintptr(y))
}

// updateCaptureFrame draws a downsampled preview of frame into the canvas.
func (a *app) updateCaptureFrame(frame *image.RGBA) {
	if frame == nil {
		return
	}
	// Determine maximum preview size based on window dimensions, leave some margin for other widgets.
	maxW := a.width - 40
	if maxW < 100 {
		maxW = 100
	}
	maxH := a.height - 140 // account for buttons/text; adjust as needed
	if maxH < 100 {
		maxH = 100
	}

	scaled := scaleToFit(frame, maxW, maxH)
	// Lazily create preview label if it doesn't exist yet (in case capture started after first frame arrives).
	if a.captureFrame == nil {
		pngBytes := encodePNG(scaled)
		a.captureFrame = Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
		Pack(a.captureFrame, Padx("1m"), Pady("1m"))
		return
	}
	// Update the existing label's image without recreating the widget.
	pngBytes := encodePNG(scaled)
	a.captureFrame.Configure(Image(NewPhoto(Data(pngBytes))))
}

// encodePNG converts an image.Image to PNG bytes. On error it returns an empty slice.
func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // ignore error; result may be empty
	return buf.Bytes()
}

// scaleToFit returns a new image scaled with nearest-neighbor so that both width and height
// fit within maxW/maxH (maintaining aspect ratio). If already within bounds it returns original.
func scaleToFit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= maxW && h <= maxH {
		return src // no scaling needed
	}
	ratioW := float64(maxW) / float64(w)
	ratioH := float64(maxH) / float64(h)
	ratio := ratioW
	if ratioH < ratio {
		ratio = ratioH
	}
	newW := int(float64(w)*ratio + 0.5)
	newH := int(float64(h)*ratio + 0.5)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		sy := int(float64(y) * float64(h) / float64(newH))
		for x := 0; x < newW; x++ {
			sx := int(float64(x) * float64(w) / float64(newW))
			c := src.At(b.Min.X+sx, b.Min.Y+sy)
			// Preserve alpha if present
			r, g, bl, a := c.RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}
