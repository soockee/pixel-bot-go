package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"strconv"
	"strings"
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
	config    *config.Config
	logger    *slog.Logger
	width     int
	height    int
	start     time.Time // app start (still used for scheduling only)
	afterID   string
	text      *TextWidget // session capture-active duration (MM:SS)
	totalText *TextWidget // total accumulated capture time (MM:SS)

	// Layout bookkeeping
	captureRow int // dedicated row for capture preview (below config panel)

	// Per-key config widgets
	minScaleText    *TextWidget
	maxScaleText    *TextWidget
	scaleStepText   *TextWidget
	thresholdText   *TextWidget
	strideText      *TextWidget
	stopOnScoreText *TextWidget
	refineText      *TextWidget // true/false
	useRGBText      *TextWidget // true/false
	returnBestText  *TextWidget // true/false
	applyBtn        *ButtonWidget

	// Debug widget for last target detection status
	debugLabel *LabelWidget

	// Capture / detection state
	captureEnabled atomic.Bool
	frameCh        chan *image.RGBA
	targetImg      image.Image
	lastDetectX    int
	lastDetectY    int
	lastDetectOK   atomic.Bool

	captureFrame *LabelWidget

	// Capture duration tracking
	captureStart        time.Time     // time when capture last toggled ON (session start)
	accumulated         time.Duration // total active capture time across completed sessions
	lastWasCapturing    bool          // previous state to detect OFF transitions
	lastSessionDuration time.Duration // duration of most recently completed session
}

func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger) *app {
	a := &app{config: cfg, logger: logger}

	App.WmTitle("")
	if a.logger != nil {
		a.logger.Info("initial config", "config", *cfg)
	}
	a.width = width
	a.height = height
	//Init target
	if img, err := assets.FishingTargetImage(); err == nil {
		a.targetImg = img
	}

	WmProtocol(App, "WM_DELETE_WINDOW", a.exitHandler)
	WmGeometry(App, fmt.Sprintf("%dx%d+100+100", width, height))
	return a
}

func (a *app) Start() {
	if a.config == nil {
		a.config = config.DefaultConfig()
	}

	// Row 0: session time, total time, debug label, buttons frame
	a.text = Text(Height(1), Width(14))
	Grid(a.text, Row(0), Column(0), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	a.totalText = Text(Height(1), Width(14))
	Grid(a.totalText, Row(0), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.3m"))

	a.debugLabel = Label(Txt("Target: <none>"), Borderwidth(1), Relief("ridge"))
	Grid(a.debugLabel, Row(0), Column(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))

	btnFrame := Frame()
	Grid(btnFrame, Row(0), Column(3), Sticky("ne"), Padx("0.3m"), Pady("0.3m"))
	// Button frame uses default column sizing.
	captureBtn := Button(Txt("Toggle Capture"), Command(func() { a.toggleCapture() }))
	Grid(captureBtn, In(btnFrame), Row(0), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	exitBtn := Button(Txt("Exit"), Command(a.exitHandler))
	Grid(exitBtn, In(btnFrame), Row(1), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))

	// Row 1+: configuration panel (columns 0-1)
	endRow := a.buildConfigPanelGrid(1) // returns next free row after config
	a.captureRow = endRow

	// Pre-allocate a stable placeholder capture preview spanning all columns so layout doesn't jump later.
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := encodePNG(placeholder)
	a.captureFrame = Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
	Grid(a.captureFrame, Row(a.captureRow), Column(0), Columnspan(4), Sticky("we"), Padx("0.4m"), Pady("0.4m"))

	a.setConfigEditable(true)

	// Initialize timer and start update loop
	a.start = time.Now()
	a.scheduleUpdate()

	App.Wait()
}

func (a *app) update() {
	// Session and total capture time handling.
	capturing := a.captureEnabled.Load()
	if capturing {
		if !a.lastWasCapturing { // session start
			a.captureStart = time.Now()
			a.lastSessionDuration = 0
		}
		a.lastSessionDuration = time.Since(a.captureStart)
	} else if a.lastWasCapturing { // session end
		// finalize session
		a.lastSessionDuration = time.Since(a.captureStart)
		a.accumulated += a.lastSessionDuration
	}
	a.lastWasCapturing = capturing

	session := a.lastSessionDuration
	total := a.accumulated
	if capturing { // include ongoing time in total display
		total = a.accumulated + session
	}

	sessMin := int(session.Minutes())
	sessSec := int(session.Seconds()) % 60
	totMin := int(total.Minutes())
	totSec := int(total.Seconds()) % 60
	if a.text != nil {
		func() {
			defer func() { _ = recover() }()
			a.text.Delete("1.0", END)
			a.text.Insert("1.0", fmt.Sprintf("Session: %02d:%02d", sessMin, sessSec))
		}()
	}
	if a.totalText != nil {
		func() {
			defer func() { _ = recover() }()
			a.totalText.Delete("1.0", END)
			a.totalText.Insert("1.0", fmt.Sprintf("Total: %02d:%02d", totMin, totSec))
		}()
	}

	// Non-blocking receive of latest frame and attempt detection.
	if a.captureEnabled.Load() && a.targetImg != nil && a.frameCh != nil {
		select {
		case frame := <-a.frameCh:
			x, y, ok := capture.DetectTemplate(frame, a.targetImg, a.config)
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

// populateConfigText writes current config values into cfgText.
// buildConfigPanel creates per-parameter single-line Text widgets and Apply button.
func (a *app) buildConfigPanelGrid(startRow int) int {
	c := a.config
	row := startRow
	makeRow := func(label, value string, target **TextWidget) {
		lbl := Label(Txt(label), Anchor("w"))
		Grid(lbl, Row(row), Column(0), Sticky("w"), Padx("0.4m"), Pady("0.15m"))
		w := Text(Height(1), Width(16))
		Grid(w, Row(row), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.15m"))
		// Column weight omitted (API may differ); relying on natural expansion.
		func() { defer func() { _ = recover() }(); w.Delete("1.0", END); w.Insert("1.0", value) }()
		*target = w
		row++
	}
	makeRow("Min Scale", fmt.Sprintf("%.2f", c.MinScale), &a.minScaleText)
	makeRow("Max Scale", fmt.Sprintf("%.2f", c.MaxScale), &a.maxScaleText)
	makeRow("Scale Step", fmt.Sprintf("%.3f", c.ScaleStep), &a.scaleStepText)
	makeRow("Threshold", fmt.Sprintf("%.3f", c.Threshold), &a.thresholdText)
	makeRow("Stride", fmt.Sprintf("%d", c.Stride), &a.strideText)
	makeRow("Stop On Score", fmt.Sprintf("%.3f", c.StopOnScore), &a.stopOnScoreText)
	makeRow("Refine (true/false)", fmt.Sprintf("%t", c.Refine), &a.refineText)
	makeRow("Use RGB (true/false)", fmt.Sprintf("%t", c.UseRGB), &a.useRGBText)
	makeRow("Return Best Even (true/false)", fmt.Sprintf("%t", c.ReturnBestEven), &a.returnBestText)
	a.applyBtn = Button(Txt("Apply Changes"), Command(func() { a.applyConfigFromWidgets() }))
	Grid(a.applyBtn, Row(row), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	row++
	return row
}

// applyConfigFromWidgets updates config from individual widgets (only if capture OFF).
func (a *app) applyConfigFromWidgets() {
	if a.captureEnabled.Load() {
		a.appendStatus("Cannot apply while capture ON")
		return
	}
	cfg := *a.config
	parseF := func(w *TextWidget, dst *float64) {
		if w == nil {
			return
		}
		v := strings.TrimSpace(a.safeGetText(w))
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dst = f
		}
	}
	parseI := func(w *TextWidget, dst *int) {
		if w == nil {
			return
		}
		v := strings.TrimSpace(a.safeGetText(w))
		if i, err := strconv.Atoi(v); err == nil {
			*dst = i
		}
	}
	parseB := func(w *TextWidget, dst *bool) {
		if w == nil {
			return
		}
		v := strings.ToLower(strings.TrimSpace(a.safeGetText(w)))
		if v == "true" || v == "1" {
			*dst = true
		} else if v == "false" || v == "0" {
			*dst = false
		}
	}
	parseF(a.minScaleText, &cfg.MinScale)
	parseF(a.maxScaleText, &cfg.MaxScale)
	parseF(a.scaleStepText, &cfg.ScaleStep)
	parseF(a.thresholdText, &cfg.Threshold)
	parseI(a.strideText, &cfg.Stride)
	parseF(a.stopOnScoreText, &cfg.StopOnScore)
	parseB(a.refineText, &cfg.Refine)
	parseB(a.useRGBText, &cfg.UseRGB)
	parseB(a.returnBestText, &cfg.ReturnBestEven)
	_ = cfg.Validate()
	*a.config = cfg
	if a.logger != nil {
		a.logger.Info("config applied", "config", cfg)
	}
	a.appendStatus("Config applied")
}

// setConfigEditable enables/disables all config widgets.
func (a *app) setConfigEditable(enabled bool) {
	state := "disabled"
	if enabled {
		state = "normal"
	}
	set := func(w *TextWidget) {
		if w != nil {
			func() { defer func() { _ = recover() }(); w.Configure(State(state)) }()
		}
	}
	set(a.minScaleText)
	set(a.maxScaleText)
	set(a.scaleStepText)
	set(a.thresholdText)
	set(a.strideText)
	set(a.stopOnScoreText)
	set(a.refineText)
	set(a.useRGBText)
	set(a.returnBestText)
	if a.applyBtn != nil {
		func() { defer func() { _ = recover() }(); a.applyBtn.Configure(State(state)) }()
	}
}

func (a *app) appendStatus(msg string) {
	if a.text == nil {
		return
	}
	func() { defer func() { _ = recover() }(); a.text.Insert("end", "\n"+msg) }()
}

func (a *app) safeGetText(t *TextWidget) string {
	if t == nil {
		return ""
	}
	var out []string
	func() { defer func() { _ = recover() }(); out = t.Get("1.0", END) }()
	return strings.Join(out, "")
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
		// Reset preview to placeholder instead of destroying to keep layout stable.
		if a.captureFrame != nil {
			placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
			pngBytes := encodePNG(placeholder)
			func() { defer func() { _ = recover() }(); a.captureFrame.Configure(Image(NewPhoto(Data(pngBytes)))) }()
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
		// Re-enable config editing
		a.setConfigEditable(true)
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
	// Disable config editing while running
	a.setConfigEditable(false)
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
	// Update the existing label's image (already allocated during Start).
	pngBytes := encodePNG(scaled)
	if a.captureFrame != nil {
		func() { defer func() { _ = recover() }(); a.captureFrame.Configure(Image(NewPhoto(Data(pngBytes)))) }()
	}
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
