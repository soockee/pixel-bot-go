package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"

	"github.com/soocke/pixel-bot-go/assets"
	"github.com/soocke/pixel-bot-go/capture"
	"github.com/soocke/pixel-bot-go/config"
)

const (
	tick = 100 * time.Millisecond
)

type app struct {
	config           *config.Config
	logger           *slog.Logger
	configPath       string // path for persistence (pixle_bot_config.json)
	width            int
	height           int
	start            time.Time // app start (still used for scheduling only)
	afterID          string
	sessionText      *TextWidget // session capture-active duration (MM:SS)
	totalSessionText *TextWidget // total accumulated capture time (MM:SS)

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
	reelKeyText     *TextWidget // reel key (e.g. F3 or R)
	// Bite detection config widgets (optional tuning)
	roiSizeText        *TextWidget
	cooldownText       *TextWidget // cooldown seconds
	maxCastDurationTxt *TextWidget // max cast duration seconds
	applyBtn           *ButtonWidget

	// Debug widget for last target detection status
	stateLabel *LabelWidget

	// Capture / detection state
	captureEnabled atomic.Bool
	frameCh        chan *image.RGBA
	targetImg      image.Image
	lastDetectX    int
	lastDetectY    int
	lastDetectOK   atomic.Bool
	detectionRect  image.Rectangle // global coordinates of current ROI used for monitoring

	captureFrame   *LabelWidget
	detectionFrame *LabelWidget // shows current ROI (detection subimage)

	selectionRect atomic.Value    // stores image.Rectangle (empty == none)
	selectionWin  *ToplevelWidget // overlay window

	// Capture duration tracking
	captureStart        time.Time     // time when capture last toggled ON (session start)
	accumulated         time.Duration // total active capture time across completed sessions
	lastWasCapturing    bool          // previous state to detect OFF transitions
	lastSessionDuration time.Duration // duration of most recently completed session

	// Fishing automation state
	fsm *FishingStateMachine
	// Bite detection (monitoring phase)
	biteDetector *BiteDetector
}

func NewApp(title string, width, height int, cfg *config.Config, logger *slog.Logger) *app {
	a := &app{config: cfg, logger: logger, configPath: "pixle_bot_config.json"}

	App.WmTitle("")
	if a.logger != nil {
		a.logger.Info("initial config", "config", *cfg)
	}
	a.width = width
	a.height = height

	if img, err := assets.FishingTargetImage(); err == nil {
		a.targetImg = img
	}
	// Restore persisted selection rectangle if present.
	if cfg != nil && cfg.SelectionW > 0 && cfg.SelectionH > 0 {
		rect := image.Rect(cfg.SelectionX, cfg.SelectionY, cfg.SelectionX+cfg.SelectionW, cfg.SelectionY+cfg.SelectionH)
		a.selectionRect.Store(rect)
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
	a.sessionText = Text(Height(1), Width(14))
	Grid(a.sessionText, Row(0), Column(0), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	a.totalSessionText = Text(Height(1), Width(14))
	Grid(a.totalSessionText, Row(0), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.3m"))

	// At startup capture isn't active yet; show <none> until user toggles capture.
	a.stateLabel = Label(Txt("State: <none>"), Borderwidth(1), Relief("ridge"))
	Grid(a.stateLabel, Row(0), Column(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))

	btnFrame := Frame()
	Grid(btnFrame, Row(0), Column(4), Sticky("ne"), Padx("0.3m"), Pady("0.3m"))
	// Button frame uses default column sizing.
	captureBtn := Button(Txt("Toggle Capture"), Command(func() { a.toggleCapture() }))
	Grid(captureBtn, In(btnFrame), Row(0), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	selectionBtn := Button(Txt("Selection Grid"), Command(func() { a.openSelectionWindow() }))
	Grid(selectionBtn, In(btnFrame), Row(1), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	exitBtn := Button(Txt("Exit"), Command(a.exitHandler))
	Grid(exitBtn, In(btnFrame), Row(2), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))

	// Row 1+: configuration panel (columns 0-1)
	endRow := a.buildConfigPanelGrid(1) // returns next free row after config
	a.captureRow = endRow

	// Pre-allocate stable placeholders so layout doesn't jump later.
	placeholder := image.NewRGBA(image.Rect(0, 0, 200, 120))
	pngBytes := encodePNG(placeholder)
	a.captureFrame = Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
	a.detectionFrame = Label(Image(NewPhoto(Data(pngBytes))), Borderwidth(1), Relief("sunken"))
	// Capture frame spans config columns 0-3; detection ROI preview sits at column 4.
	Grid(a.captureFrame, Row(a.captureRow), Column(0), Columnspan(4), Sticky("we"), Padx("0.4m"), Pady("0.4m"))
	Grid(a.detectionFrame, Row(a.captureRow), Column(4), Columnspan(1), Sticky("we"), Padx("0.4m"), Pady("0.4m"))

	a.setConfigEditable(true)

	// Initialize timer and start update loop
	a.start = time.Now()

	// Initialize fishing state machine using configured cooldown.
	a.fsm = NewFishingStateMachine(a.logger, a.config)

	// Update state label synchronously on state changes.
	a.fsm.AddListener(func(prev, next FishingState) {
		if a.stateLabel == nil {
			return
		}
		if next == StateSearching {
			// Show target search info if currently searching.
			a.stateLabel.Configure(Txt("State: <searching>"))
			return
		}
		if next == StateMonitoring {
			// Initialize bite detector on monitoring entry.
			a.biteDetector = NewBiteDetector(a.config, a.logger)
			a.biteDetector.Reset()
		}
		a.stateLabel.Configure(Txt(fmt.Sprintf("State: %s", next)))
	})
	a.scheduleUpdate()

	App.Wait()
}

func (a *app) update() {
	// Session and total capture time handling.
	if a.fsm != nil { // advance FSM timers (cooldown -> casting)
		a.fsm.Tick(time.Now())
	}
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
	if a.sessionText != nil {
		a.sessionText.Delete("1.0", END)
		a.sessionText.Insert("1.0", fmt.Sprintf("Session: %02d:%02d", sessMin, sessSec))
	}
	if a.totalSessionText != nil {
		a.totalSessionText.Delete("1.0", END)
		a.totalSessionText.Insert("1.0", fmt.Sprintf("Total: %02d:%02d", totMin, totSec))
	}

	// Frame processing depending on FSM state.
	fsmState := StateSearching
	if a.fsm != nil {
		fsmState = a.fsm.Current()
	}
	if a.captureEnabled.Load() && a.frameCh != nil {
		select {
		case frame := <-a.frameCh:
			// Always update preview regardless of state.
			a.updateCaptureFrame(frame)
			if fsmState == StateSearching && a.targetImg != nil {
				// Template detection path.
				x, y, ok := capture.DetectTemplate(frame, a.targetImg, a.config)
				if ok {
					if sel := a.getSelectionRect(); sel != nil {
						x += sel.Min.X
						y += sel.Min.Y
					}
					a.lastDetectX, a.lastDetectY = x, y
					a.lastDetectOK.Store(true)
					moveCursor(x, y)
					if a.fsm != nil {
						a.fsm.EventTargetAcquiredAt(x, y)
					}
				} else {
					a.lastDetectOK.Store(false)
				}
			} else if fsmState == StateMonitoring && a.biteDetector != nil {
				x, y, ok := a.fsm.TargetCoordinates()
				if ok {
					// Adjust for selection subset if active (coordinates stored absolute)
					if sel := a.getSelectionRect(); sel != nil {
						x -= sel.Min.X
						y -= sel.Min.Y
					}
					// Desired ROI size
					w := a.config.ROISizePx
					h := a.config.ROISizePx
					if w < 1 {
						w = 1
					}
					if h < 1 {
						h = 1
					}
					// Center ROI around (x,y)
					dx := x - w/2
					dy := y - h/2
					fb := frame.Bounds()
					// Clip to frame bounds
					if dx < 0 {
						dx = 0
					}
					if dy < 0 {
						dy = 0
					}
					if dx+w > fb.Dx() {
						w = fb.Dx() - dx
					}
					if dy+h > fb.Dy() {
						h = fb.Dy() - dy
					}
					if w < 1 {
						w = 1
					}
					if h < 1 {
						h = 1
					}
					roiRect := image.Rect(dx, dy, dx+w, dy+h)
					// Store global coordinates version for debugging/visualization
					if sel := a.getSelectionRect(); sel != nil {
						// selection frame already cropped; add selection offset
						a.detectionRect = image.Rect(sel.Min.X+roiRect.Min.X, sel.Min.Y+roiRect.Min.Y, sel.Min.X+roiRect.Max.X, sel.Min.Y+roiRect.Max.Y)
					} else {
						a.detectionRect = roiRect
					}
					// Obtain subimage
					sub := frame.SubImage(roiRect)
					var roiImg *image.RGBA
					if rgba, ok2 := sub.(*image.RGBA); ok2 {
						roiImg = rgba
					} else {
						// Fallback copy if underlying type differs
						roiImg = image.NewRGBA(image.Rect(0, 0, roiRect.Dx(), roiRect.Dy()))
						draw.Draw(roiImg, roiImg.Bounds(), sub, roiRect.Min, draw.Src)
					}
					// Update detection preview widget
					if a.detectionFrame != nil {
						roiPNG := encodePNG(roiImg)
						a.detectionFrame.Configure(Image(NewPhoto(Data(roiPNG))))
					}
					// Feed ROI frame to bite detector
					if a.biteDetector.FeedFrame(roiImg, time.Now()) {
						if a.fsm != nil {
							a.fsm.EventFishBite()
						}
					} else if a.biteDetector.TargetLostHeuristic() {
						if a.fsm != nil {
							a.fsm.EventTargetLost()
						}
					}
				}
			}
		default:
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
		w.Delete("1.0", END)
		w.Insert("1.0", value)
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
	makeRow("Reel Key (e.g. F3 or R)", c.ReelKey, &a.reelKeyText)
	makeRow("ROI Size Px", fmt.Sprintf("%d", c.ROISizePx), &a.roiSizeText)
	makeRow("Cooldown Seconds", fmt.Sprintf("%d", c.CooldownSeconds), &a.cooldownText)
	makeRow("Max Cast Duration Seconds", fmt.Sprintf("%d", c.MaxCastDurationSeconds), &a.maxCastDurationTxt)
	a.applyBtn = Button(Txt("Apply Changes"), Command(func() { a.applyConfigFromWidgets() }))
	Grid(a.applyBtn, Row(row), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	row++
	return row
}

// applyConfigFromWidgets updates config from individual widgets (only if capture OFF).
func (a *app) applyConfigFromWidgets() {
	if a.captureEnabled.Load() {
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
		switch v {
		case "true", "1":
			*dst = true
		case "false", "0":
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
	parseI(a.cooldownText, &cfg.CooldownSeconds)
	parseI(a.maxCastDurationTxt, &cfg.MaxCastDurationSeconds)
	// Reel key string (allow raw token like F3 or single letter)
	if a.reelKeyText != nil {
		val := strings.TrimSpace(a.safeGetText(a.reelKeyText))
		if val != "" {
			cfg.ReelKey = val
		}
	}
	_ = cfg.Validate()
	*a.config = cfg
	if a.logger != nil {
		a.logger.Info("config applied", "config", cfg)
	}
	// Persist full config to JSON after application.
	if err := a.config.Save(a.configPath); err != nil {
		if a.logger != nil {
			a.logger.Error("config save failed", "error", err)
		}
	} else if a.logger != nil {
		a.logger.Info("config saved", "path", a.configPath)
	}
}

// setConfigEditable enables/disables all config widgets.
func (a *app) setConfigEditable(enabled bool) {
	state := "disabled"
	if enabled {
		state = "normal"
	}
	set := func(w *TextWidget) {
		if w != nil {
			w.Configure(State(state))
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
	set(a.roiSizeText)
	set(a.cooldownText)
	set(a.maxCastDurationTxt)
	if a.applyBtn != nil {
		a.applyBtn.Configure(State(state))
	}
}

func (a *app) safeGetText(t *TextWidget) string {
	if t == nil {
		return ""
	}
	out := t.Get("1.0", END)
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

// openSelectionWindow opens a resizable/movable top-level window the user can position
// and resize. Pressing Confirm stores the selected rectangle; Cancel dismisses it.
// If a selection rectangle is already active, clicking the button clears it and
// reverts to full-screen capture.
func (a *app) openSelectionWindow() {
	// If already open bring to front.
	if a.selectionWin != nil {
		WmGeometry(a.selectionWin.Window)
		return
	}

	win := App.Toplevel(Borderwidth(2), Background("#008080"))
	win.WmTitle("Selection Grid")
	a.selectionWin = win
	// Compute centered geometry: 2/3 of primary screen width & height.
	user32 := windows.NewLazySystemDLL("user32.dll")
	getSystemMetrics := user32.NewProc("GetSystemMetrics")
	cx, _, _ := getSystemMetrics.Call(uintptr(0)) // SM_CXSCREEN
	cy, _, _ := getSystemMetrics.Call(uintptr(1)) // SM_CYSCREEN
	screenW := int(cx)
	screenH := int(cy)
	initW := screenW * 2 / 3
	initH := screenH * 5 / 9
	if initW < 1 {
		initW = 1
	}
	if initH < 1 {
		initH = 1
	}
	x := (screenW - initW) / 2
	y := (screenH - initH) / 2
	WmGeometry(win.Window, fmt.Sprintf("%dx%d+%d+%d", initW, initH, x, y))

	WmAttributes(win.Window, "-topmost", 1)
	WmAttributes(win.Window, "-toolwindow", true)
	// Keyed transparency: any pixel painted exactly in this hex teal becomes transparent (Win2000/XP+).
	WmAttributes(win.Window, "-transparentcolor", "#008080")

	a.logger.Debug("wm attributes", slog.String("event", "open selection window"), slog.Any("details", WmAttributes(win.Window)))

	// Layout with left/right border frames for clearer visual bounds.
	// Row 0: three columns -> left border | center (selection area) | right border.
	GridRowConfigure(win.Window, 0, Weight(1))
	GridColumnConfigure(win.Window, 0, Weight(0)) // left border fixed
	GridColumnConfigure(win.Window, 1, Weight(1)) // center expands
	GridColumnConfigure(win.Window, 2, Weight(0)) // right border fixed

	leftBorder := win.Frame(Width(4), Background("#FFFFFF")) // vivid red
	Grid(leftBorder, Row(0), Column(0), Sticky("ns"))
	centerArea := win.Frame(Background("#008080")) // same teal; becomes transparent
	Grid(centerArea, Row(0), Column(1), Sticky("nsew"))
	rightBorder := win.Frame(Width(4), Background("#FFFFFF"))
	Grid(rightBorder, Row(0), Column(2), Sticky("ns"))

	controls := win.Frame()
	Grid(controls, Row(1), Column(0), Columnspan(3), Sticky("we"))
	confirm := win.Button(Txt("Confirm [Enter]"), Command(a.confirmSelection))
	Grid(confirm, In(controls), Row(0), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	cancel := win.Button(Txt("Cancel [Esc]"), Command(a.cancelSelectionWindow))
	Grid(cancel, In(controls), Row(0), Column(1), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	clear := win.Button(Txt("Clear"), Command(a.clearSelection))
	Grid(clear, In(controls), Row(0), Column(2), Sticky("we"), Padx("0.2m"), Pady("0.2m"))

	// Bind keys to the toplevel widget itself (not its underlying Window field) to avoid invalid command references.
	Bind(win, "<Return>", Command(a.confirmSelection))
	Bind(win, "<Escape>", Command(a.cancelSelectionWindow))
}

// clearSelection resets selection (in-memory + persisted) without opening window.
func (a *app) clearSelection() {
	a.selectionRect.Store(image.Rectangle{})
	if a.config != nil {
		a.config.SelectionW = 0
		a.config.SelectionH = 0
		_ = a.config.Save(a.configPath)
	}
}

// redrawOverlay repaints translucent rectangle filling entire window client area.
// redrawOverlay removed: entire window now serves as the tinted selection area.

// confirmSelection reads geometry of selectionWin, stores rectangle, destroys window.
func (a *app) confirmSelection() {
	if a.selectionWin == nil {
		return
	}
	geom := WmGeometry(a.selectionWin.Window)
	if rect, ok := parseGeometry(geom); ok {
		a.selectionRect.Store(rect)
		// Persist selection to config
		if a.config != nil {
			a.config.SelectionX = rect.Min.X
			a.config.SelectionY = rect.Min.Y
			a.config.SelectionW = rect.Dx()
			a.config.SelectionH = rect.Dy()
			_ = a.config.Save(a.configPath)
		}
	}
	a.destroySelectionWindow()
}

// cancelSelectionWindow discards window without changing selection.
func (a *app) cancelSelectionWindow() {
	a.destroySelectionWindow()
}

func (a *app) destroySelectionWindow() {
	if a.selectionWin != nil {
		Destroy(a.selectionWin)
		a.selectionWin = nil
	}
}

// parseGeometry parses Tk WM geometry strings: WxH+X+Y
var geomRe = regexp.MustCompile(`^(\d+)x(\d+)\+(-?\d+)\+(-?\d+)$`)

func parseGeometry(g string) (image.Rectangle, bool) {
	g = strings.TrimSpace(g)
	m := geomRe.FindStringSubmatch(g)
	if len(m) != 5 {
		return image.Rectangle{}, false
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	x, _ := strconv.Atoi(m[3])
	y, _ := strconv.Atoi(m[4])
	if w <= 0 || h <= 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(x, y, x+w, y+h), true
}

// getSelectionRect returns the stored rectangle or nil if none.
func (a *app) getSelectionRect() *image.Rectangle {
	v := a.selectionRect.Load()
	if v == nil {
		return nil
	}
	r, ok := v.(image.Rectangle)
	if !ok {
		return nil
	}
	if r == (image.Rectangle{}) { // sentinel for cleared selection
		return nil
	}
	return &r
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
			a.captureFrame.Configure(Image(NewPhoto(Data(pngBytes))))
			if a.detectionFrame != nil {
				a.detectionFrame.Configure(Image(NewPhoto(Data(pngBytes))))
			}
		}
		// Reset debug widget state
		if a.stateLabel != nil {
			a.stateLabel.Configure(Txt("State: <none>"))
		}
		// Gracefully reset FSM & bite detector when capture stops.
		if a.fsm != nil {
			a.fsm.Reset()
		}
		a.biteDetector = nil
		// Clear last detection status
		a.lastDetectOK.Store(false)

		// Re-enable config editing
		a.setConfigEditable(true)
		return
	}
	// Enable capture
	if a.frameCh == nil {
		a.frameCh = make(chan *image.RGBA, 1)
	}
	a.captureEnabled.Store(true)
	// Reflect current FSM state now that detection lifecycle begins.
	if a.stateLabel != nil && a.fsm != nil {
		cur := a.fsm.Current()
		switch cur {
		case StateSearching:
			a.stateLabel.Configure(Txt("State: <searching>"))
		case StateMonitoring:
			a.stateLabel.Configure(Txt("State: monitoring"))
		default:
			a.stateLabel.Configure(Txt(fmt.Sprintf("State: %s", cur.String())))
		}
	}

	go a.captureLoop()
	// Disable config editing while running
	a.setConfigEditable(false)
}

// captureLoop runs in a goroutine and pushes latest frames.
func (a *app) captureLoop() {
	for a.captureEnabled.Load() {
		var img *image.RGBA
		if rect := a.getSelectionRect(); rect != nil {
			// Use selected sub-rectangle
			partial, err := capture.GrabSelection(*rect)
			if err == nil && partial != nil {
				img = partial
			}
		} else {
			full, err := capture.Grab()
			if err == nil && full != nil {
				img = full
			}
		}
		if img != nil {
			select {
			case a.frameCh <- img:
			default:
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

// pressKey issues a key down + key up for the given virtual-key code (Windows only).
// This uses keybd_event for simplicity; for production consider SendInput.
func pressKey(vk byte) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	keybdEvent := user32.NewProc("keybd_event")
	const KEYEVENTF_KEYUP = 0x0002
	// key down
	_, _, _ = keybdEvent.Call(uintptr(vk), 0, 0, 0)
	// small sleep to emulate human press duration
	time.Sleep(40 * time.Millisecond)
	// key up
	_, _, _ = keybdEvent.Call(uintptr(vk), 0, KEYEVENTF_KEYUP, 0)
}

// performCastAction issues the configured cast key (currently reusing ReelKey until a dedicated CastKey exists).
// It is executed in its own goroutine to avoid blocking the Tk event loop.
// performCastAction deprecated: logic inlined into FSM.

// performReelAction executes the reel sequence asynchronously.
// It repositions cursor, right-clicks, then sends the configured reel key.
// Separation prevents blocking the UI update thread during sleeps or Windows API calls.
func (a *app) performReelAction() {
	if a.fsm == nil {
		return
	}
	x, y, ok := a.fsm.TargetCoordinates()
	if !ok {
		if a.logger != nil {
			a.logger.Info("reel action skipped - no target coords")
		}
		return
	}
	moveCursor(x, y)
	clickRight()
	vk := parseVK(a.config.ReelKey)
	pressKey(vk)
	if a.logger != nil {
		a.logger.Info("reel action", "x", x, "y", y, "mouse", "right+key")
	}
	// Optional: briefly pause bite detector after trigger to avoid re-trigger spam.
	a.biteDetector = nil
}

// clickRight performs a right mouse button click (down + up) using legacy mouse_event.
// For production use, SendInput is preferred for synthesis reliability.
func clickRight() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	mouseEvent := user32.NewProc("mouse_event")
	const MOUSEEVENTF_RIGHTDOWN = 0x0008
	const MOUSEEVENTF_RIGHTUP = 0x0010
	_, _, _ = mouseEvent.Call(MOUSEEVENTF_RIGHTDOWN, 0, 0, 0, 0)
	time.Sleep(30 * time.Millisecond)
	_, _, _ = mouseEvent.Call(MOUSEEVENTF_RIGHTUP, 0, 0, 0, 0)
}

// parseVK converts a user-provided key token (e.g. "F3", "R") into a Windows virtual key code.
// Supports function keys F1-F12 and single alphabetic characters. Falls back to F3 if unknown.
func parseVK(key string) byte {
	k := strings.ToUpper(strings.TrimSpace(key))
	if len(k) == 2 && k[0] == 'F' { // F1-F9
		n := int(k[1] - '0')
		if n >= 1 && n <= 9 {
			return byte(0x70 + (n - 1)) // VK_F1=0x70
		}
	}
	if len(k) == 3 && k[0] == 'F' { // F10-F12
		switch k {
		case "F10":
			return 0x79
		case "F11":
			return 0x7A
		case "F12":
			return 0x7B
		}
	}
	if len(k) == 2 && k[0] == 'F' { // F10-F19 (optional) -> ignore beyond F12 for now
		// fallthrough
	}
	if len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z' {
		return k[0] // 'A'..'Z' match VK codes
	}
	// Default fallback F3
	return 0x72
}

// updateCaptureFrame draws a downsampled preview of frame into the canvas.
func (a *app) updateCaptureFrame(frame *image.RGBA) {
	if frame == nil {
		return
	}
	maxW := a.width - 40
	if maxW < 100 {
		maxW = 100
	}
	maxH := a.height - 140
	if maxH < 100 {
		maxH = 100
	}

	scaled := scaleToFit(frame, maxW, maxH)
	pngBytes := encodePNG(scaled)
	if a.captureFrame != nil {
		a.captureFrame.Configure(Image(NewPhoto(Data(pngBytes))))
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
