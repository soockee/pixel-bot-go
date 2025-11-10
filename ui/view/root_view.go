package view

import (
	"image"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/ui/images"
	"github.com/soocke/pixel-bot-go/ui/theme"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// RootView composes the top-level application layout and wires UI callbacks.
// It owns high-level subviews but exposes minimal exported fields for presenters.
type RootView struct {
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger

	// Subviews
	Session     SessionStats
	ConfigPanel ConfigPanel
	CapturePrev CapturePreview

	// Widgets
	StateLabel       *TLabelWidget
	WindowSelect     *TComboboxWidget
	StatusLabel      *LabelWidget
	sessionLabel     *TLabelWidget
	totalLabel       *TLabelWidget
	windowExplainLbl *TLabelWidget
	captureLabel     *LabelWidget
	detectionLabel   *LabelWidget
	captureBtn       *ButtonWidget
	selectionBtn     *ButtonWidget
	exitBtn          *ButtonWidget
	captureRow       int
	// Layout containers we may rebuild
	configFrame     *FrameWidget
	mainFrame       *FrameWidget
	headerFrame     *FrameWidget
	leftInlineFrame *FrameWidget
	actionsFrame    *FrameWidget
	statusBarFrame  *FrameWidget
	configVisible   bool
	toggleConfigBtn *ButtonWidget
	scaleBound      bool
	darkMode        bool
	darkToggleBtn   *ButtonWidget
}

// UI abstracts the subset of view operations needed by presenters, enabling decoupling
// from the concrete RootView implementation.
type UI interface {
	SetStateLabel(text string)
	SetConfigEditable(enabled bool)
	UpdateCapture(img image.Image)
	UpdateDetection(img image.Image)
	SetSession(seconds int, totalSeconds int)
}

func NewRootView(cfg *config.Config, cfgPath string, logger *slog.Logger) *RootView {
	return &RootView{cfg: cfg, cfgPath: cfgPath, logger: logger}
}

// Build constructs the layout. titles: list of window titles for selection dropdown.
// Handlers are invoked on user actions.
func (rv *RootView) Build(titles []string, onToggleCapture func(), onSelectionGrid func(), onExit func(), onWindowChanged func(title string)) {
	if rv == nil {
		return
	}
	// Initialize styles once (idempotent if called multiple times in current session).
	theme.InitStyles()
	// Apply persisted dark mode preference before constructing palette-dependent widgets.
	if rv.cfg != nil && rv.cfg.DarkMode {
		theme.SetDark(true)
		rv.darkMode = true
	}

	// --- Modern Layout Structure ---
	// Grid the App root: header (row0), body (row1), status (row2)
	GridRowConfigure(App, 0, Weight(0))
	GridRowConfigure(App, 1, Weight(1))
	GridRowConfigure(App, 2, Weight(0))
	GridColumnConfigure(App, 0, Weight(0)) // side panel
	GridColumnConfigure(App, 1, Weight(1)) // main content

	pal := theme.CurrentPalette()

	// Header Frame (with sub-frame for toggle + timers stacked)
	rv.headerFrame = Frame(Background(pal.Surface), Borderwidth(0))
	Grid(rv.headerFrame, Row(0), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	GridColumnConfigure(rv.headerFrame, 0, Weight(0)) // left stack
	GridColumnConfigure(rv.headerFrame, 1, Weight(1)) // actions stretch
	GridColumnConfigure(rv.headerFrame, 2, Weight(0)) // state label

	// Left inline frame: toggle + timers on same row
	rv.leftInlineFrame = Frame(Background(pal.Surface), Borderwidth(0))
	Grid(rv.leftInlineFrame, In(rv.headerFrame), Row(0), Column(0), Sticky("nw"))
	GridColumnConfigure(rv.leftInlineFrame, 0, Weight(0)) // toggle
	GridColumnConfigure(rv.leftInlineFrame, 1, Weight(0)) // session
	GridColumnConfigure(rv.leftInlineFrame, 2, Weight(0)) // total

	rv.Session = NewSessionStats(0)
	rv.sessionLabel = TLabel(Txt("Session: 00:00"))
	rv.totalLabel = TLabel(Txt("Total: 00:00"))
	Grid(rv.sessionLabel, In(rv.leftInlineFrame), Row(0), Column(1), Sticky("w"), Padx("0.2m"))
	Grid(rv.totalLabel, In(rv.leftInlineFrame), Row(0), Column(2), Sticky("w"), Padx("0.2m"))

	// Actions / window selection area
	rv.actionsFrame = Frame(Background(pal.Surface))
	Grid(rv.actionsFrame, In(rv.headerFrame), Row(0), Column(1), Sticky("we"))
	// Columns: 0 explanation label, 1 combobox (stretch), 2 capture btn, 3 selection btn, 4 exit btn
	GridColumnConfigure(rv.actionsFrame, 0, Weight(0))
	GridColumnConfigure(rv.actionsFrame, 1, Weight(1))
	GridColumnConfigure(rv.actionsFrame, 2, Weight(0))
	GridColumnConfigure(rv.actionsFrame, 3, Weight(0))
	GridColumnConfigure(rv.actionsFrame, 4, Weight(0))

	// State label on right
	rv.StateLabel = TLabel(Txt("State: <none>"))
	Grid(rv.StateLabel, In(rv.headerFrame), Row(0), Column(2), Sticky("e"), Padx("0.3m"))

	if len(titles) == 0 {
		titles = []string{"<none>"}
	}
	// Explanation label for window selection purpose
	rv.windowExplainLbl = TLabel(Txt("Target Window:"))
	Grid(rv.windowExplainLbl, In(rv.actionsFrame), Row(0), Column(0), Sticky("w"), Padx("0.2m"), Pady("0.2m"))
	rv.WindowSelect = TCombobox(Values(titles), Width(26))
	Grid(rv.WindowSelect, In(rv.actionsFrame), Row(0), Column(1), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.WindowSelect.Current(0)
	Bind(rv.WindowSelect, "<<ComboboxSelected>>", Command(func() {
		if rv.WindowSelect != nil {
			idxStr := rv.WindowSelect.Current(nil)
			idx, err := strconv.Atoi(idxStr)
			if err == nil && idx >= 0 && idx < len(titles) {
				onWindowChanged(titles[idx])
			} else if rv.logger != nil {
				rv.logger.Error("window selection parse error", "error", err)
			}
		}
	}))
	rv.captureBtn = Button(Txt("Toggle Capture"), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1), Command(onToggleCapture))
	Grid(rv.captureBtn, In(rv.actionsFrame), Row(0), Column(2), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.selectionBtn = Button(Txt("Selection"), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1), Command(onSelectionGrid))
	Grid(rv.selectionBtn, In(rv.actionsFrame), Row(0), Column(3), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.exitBtn = Button(Txt("Exit"), Background(pal.Danger), Foreground("white"), Relief("raised"), Borderwidth(1), Command(onExit))
	Grid(rv.exitBtn, In(rv.actionsFrame), Row(0), Column(4), Sticky("we"), Padx("0.2m"), Pady("0.2m"))

	// Body: config hidden by default; mainFrame spans both columns initially
	rv.configVisible = false
	rv.configFrame = nil
	rv.mainFrame = Frame(Background(pal.Surface), Relief("flat"))
	Grid(rv.mainFrame, Row(1), Column(0), Columnspan(2), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
	GridRowConfigure(rv.mainFrame, 0, Weight(1))
	GridColumnConfigure(rv.mainFrame, 0, Weight(1))

	// Prepare ConfigPanel (UI built only when shown)
	rv.ConfigPanel = NewConfigPanel(rv.cfg, rv.cfgPath, rv.logger)
	rv.captureRow = 0

	// Capture & Detection preview inside mainFrame
	// Provide placeholder images so widgets reserve appropriate pixel dimensions.
	// Capture placeholder (arbitrary 400x225 similar to previous constants)
	capturePh := image.NewRGBA(image.Rect(0, 0, 400, 225))
	capture := Label(Image(NewPhoto(Data(images.EncodePNG(capturePh)))), Relief("sunken"), Borderwidth(1))
	Grid(capture, In(rv.mainFrame), Row(0), Column(0), Sticky("nsew"), Padx("0.3m"), Pady("0.3m"))

	// Detection placeholder sized exactly to configured ROI (square)
	roiSize := rv.cfg.ROISizePx
	if roiSize <= 0 { // fallback safety
		roiSize = 80
	}
	detectionPh := image.NewRGBA(image.Rect(0, 0, roiSize, roiSize))
	detection := Label(Image(NewPhoto(Data(images.EncodePNG(detectionPh)))), Relief("sunken"), Borderwidth(1))
	// Place detection to the right of capture; allow natural size (no Width/Height hints)
	Grid(detection, In(rv.mainFrame), Row(0), Column(1), Sticky("n"), Padx("0.3m"), Pady("0.3m"))

	// Replace old CapturePrev with new labels hooking into same interface expectations.
	rv.CapturePrev = &capturePreview{captureLabel: capture, detectionLabel: detection}
	rv.captureLabel = capture
	rv.detectionLabel = detection
	// Provide generous initial fallback size before geometry is realized.
	if cp, ok := rv.CapturePrev.(*capturePreview); ok {
		cp.setTargetSize(800, 450)
	}
	// Bind <Configure> once to recompute scaling when window size changes (first real layout pass).
	if !rv.scaleBound {
		Bind(App, "<Configure>", Command(func() { rv.updatePreviewScale() }))
		rv.scaleBound = true
	}

	// Status bar
	rv.statusBarFrame = Frame(Background(pal.Surface))
	Grid(rv.statusBarFrame, Row(2), Column(0), Columnspan(2), Sticky("we"))
	rv.StatusLabel = Label(Txt("Ready"), Anchor("w"))
	Grid(rv.StatusLabel, In(rv.statusBarFrame), Row(0), Column(0), Sticky("w"), Padx("0.4m"), Pady("0.2m"))

	// Add config toggle button (placed after initial build so frames exist)
	rv.toggleConfigBtn = Button(Txt("Show Config"), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1),
		Command(func() { rv.toggleConfig() }))
	Grid(rv.toggleConfigBtn, In(rv.leftInlineFrame), Row(0), Column(0), Sticky("w"), Padx("0.2m"), Pady("0.1m"))

	// Dark mode toggle button
	rv.darkToggleBtn = Button(Txt("Dark Mode"), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1),
		Command(func() { rv.toggleDarkMode() }))
	Grid(rv.darkToggleBtn, In(rv.leftInlineFrame), Row(0), Column(3), Sticky("w"), Padx("0.2m"), Pady("0.1m"))

	// Apply initial palette to labels
	rv.applyPalette()
}

// SetStateLabel updates the state label text.
func (rv *RootView) SetStateLabel(text string) {
	if rv != nil && rv.StateLabel != nil {
		rv.StateLabel.Configure(Txt(text))
	}
}

// SetConfigEditable toggles config panel editability.
func (rv *RootView) SetConfigEditable(enabled bool) {
	if rv != nil && rv.ConfigPanel != nil {
		rv.ConfigPanel.SetEditable(enabled)
	}
}

// ConfigEditable implements presenter.CaptureView (alias to SetConfigEditable)
func (rv *RootView) ConfigEditable(enabled bool) { rv.SetConfigEditable(enabled) }

// UpdateCapture proxies to underlying capture preview view.
func (rv *RootView) UpdateCapture(img image.Image) {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.UpdateCapture(img)
	}
}

// UpdateDetection proxies to underlying capture preview view.
func (rv *RootView) UpdateDetection(img image.Image) {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.UpdateDetection(img)
	}
}

// SetSession updates both session and total capture durations.
func (rv *RootView) SetSession(seconds int, totalSeconds int) {
	if rv == nil || rv.Session == nil {
		return
	}
	rv.Session.SetSession(seconds)
	rv.Session.SetTotal(totalSeconds)
}

// --- CapturePresenter view contract methods ---
// PreviewReset clears the capture preview canvas.
func (rv *RootView) PreviewReset() {
	if rv != nil && rv.CapturePrev != nil {
		rv.CapturePrev.Reset()
	}
}

// toggleConfig collapses or expands the config panel, re-gridding mainFrame accordingly.
func (rv *RootView) toggleConfig() {
	if rv == nil || rv.mainFrame == nil {
		return
	}
	if rv.configVisible { // hide
		if rv.configFrame != nil {
			Destroy(rv.configFrame)
			rv.configFrame = nil
		}
		Grid(rv.mainFrame, Row(1), Column(0), Columnspan(2), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
		rv.configVisible = false
		if rv.toggleConfigBtn != nil {
			rv.toggleConfigBtn.Configure(Txt("Show Config"))
		}
	} else { // show
		rv.configFrame = Frame(Background(theme.ColorSurface), Relief("groove"), Borderwidth(1))
		Grid(rv.configFrame, Row(1), Column(0), Sticky("ns"), Padx("0.4m"), Pady("0.2m"))
		GridRowConfigure(rv.configFrame, 0, Weight(0))
		GridColumnConfigure(rv.configFrame, 0, Weight(1))
		Grid(rv.mainFrame, Row(1), Column(1), Columnspan(1), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
		// rebuild panel
		rv.ConfigPanel = NewConfigPanel(rv.cfg, rv.cfgPath, rv.logger)
		rv.captureRow = rv.ConfigPanel.Build(0, rv.configFrame)
		rv.configVisible = true
		if rv.toggleConfigBtn != nil {
			rv.toggleConfigBtn.Configure(Txt("Hide Config"))
		}
	}
	rv.updatePreviewScale()
	// Ensure palette reapplied to newly created config frame contents
	rv.applyPalette()
}

// toggleDarkMode flips theme dark/light and updates container backgrounds.
func (rv *RootView) toggleDarkMode() {
	if rv == nil {
		return
	}
	rv.darkMode = theme.ToggleDark()
	rv.applyPalette()
	// Persist preference
	if rv.cfg != nil {
		rv.cfg.DarkMode = rv.darkMode
		if err := rv.cfg.Save(rv.cfgPath); err != nil {
			if rv.logger != nil {
				rv.logger.Error("persist dark mode failed", "error", err)
			}
		} else if rv.logger != nil {
			rv.logger.Info("dark mode preference saved", "dark", rv.darkMode)
		}
	}
}

// applyPalette updates widget colors based on the current palette snapshot.
func (rv *RootView) applyPalette() {
	pal := theme.CurrentPalette()
	App.Configure(Background(pal.AppBg))
	// Frames
	if rv.headerFrame != nil {
		rv.headerFrame.Configure(Background(pal.Surface))
	}
	if rv.leftInlineFrame != nil {
		rv.leftInlineFrame.Configure(Background(pal.Surface))
	}
	if rv.actionsFrame != nil {
		rv.actionsFrame.Configure(Background(pal.Surface))
	}
	if rv.mainFrame != nil {
		rv.mainFrame.Configure(Background(pal.Surface))
	}
	if rv.statusBarFrame != nil {
		rv.statusBarFrame.Configure(Background(pal.Surface))
	}
	if rv.configFrame != nil {
		rv.configFrame.Configure(Background(pal.Surface))
	}
	// Labels
	if rv.sessionLabel != nil {
		rv.sessionLabel.Configure(Background(pal.Surface), Foreground(pal.Text))
	}
	if rv.totalLabel != nil {
		rv.totalLabel.Configure(Background(pal.Surface), Foreground(pal.Text))
	}
	if rv.windowExplainLbl != nil {
		rv.windowExplainLbl.Configure(Background(pal.Surface), Foreground(pal.TextMuted))
	}
	if rv.StateLabel != nil {
		rv.StateLabel.Configure(Background(pal.Accent), Foreground("white"))
	}
	if rv.StatusLabel != nil {
		rv.StatusLabel.Configure(Background(pal.Surface), Foreground(pal.TextMuted))
	}
	if rv.captureLabel != nil {
		rv.captureLabel.Configure(Background(pal.Surface))
	}
	if rv.detectionLabel != nil {
		rv.detectionLabel.Configure(Background(pal.Surface))
	}
	// Buttons
	if rv.toggleConfigBtn != nil {
		rv.toggleConfigBtn.Configure(Background(pal.Primary), Foreground("white"))
	}
	if rv.darkToggleBtn != nil {
		if theme.IsDark() {
			rv.darkToggleBtn.Configure(Txt("Light Mode"))
		} else {
			rv.darkToggleBtn.Configure(Txt("Dark Mode"))
		}
		rv.darkToggleBtn.Configure(Background(pal.Primary), Foreground("white"))
	}
	if rv.captureBtn != nil {
		rv.captureBtn.Configure(Background(pal.Primary), Foreground("white"))
	}
	if rv.selectionBtn != nil {
		rv.selectionBtn.Configure(Background(pal.Primary), Foreground("white"))
	}
	if rv.exitBtn != nil {
		rv.exitBtn.Configure(Background(pal.Danger), Foreground("white"))
	}
	// Config panel widgets (text entries)
	if cp, ok := rv.ConfigPanel.(*configPanel); ok {
		for _, tw := range cp.widgets {
			if tw != nil {
				if theme.IsDark() {
					tw.Configure(Background("#334155"), Foreground(pal.Text))
				} else {
					tw.Configure(Background("white"), Foreground("black"))
				}
			}
		}
		if cp.applyBtn != nil {
			cp.applyBtn.Configure(Background(pal.Primary), Foreground("white"))
		}
	}
}

// --- Geometry helpers ---
var geomReRoot = regexp.MustCompile(`^(\d+)x(\d+)\+(-?\d+)\+(-?\d+)$`)

func parseGeometry(g string) (w, h int, ok bool) {
	g = strings.TrimSpace(g)
	m := geomReRoot.FindStringSubmatch(g)
	if len(m) != 5 {
		return 0, 0, false
	}
	w, _ = strconv.Atoi(m[1])
	h, _ = strconv.Atoi(m[2])
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// updatePreviewScale recalculates capture preview target size using window geometry.
func (rv *RootView) updatePreviewScale() {
	if rv == nil || rv.CapturePrev == nil {
		return
	}
	geom := WmGeometry(App)
	w, h, ok := parseGeometry(geom)
	if !ok {
		w, h = 1280, 720 // fallback typical size if geometry not ready
	}
	// Ignore obviously uninitialized tiny geometry (Tk may report 1x1 early)
	if w < 400 || h < 300 {
		// keep previously set fallback; don't overwrite with minuscule scaling yet
		return
	}
	roiW := rv.cfg.ROISizePx
	if roiW <= 0 {
		roiW = 80
	}
	margin := 32
	configW := 0
	if rv.configVisible {
		configW = 280
	}
	usableW := w - roiW - configW - margin
	if usableW < 320 {
		usableW = 320
	}
	if usableW > w {
		usableW = w - margin
	}
	headerH := 64
	statusH := 30
	usableH := h - headerH - statusH - margin
	if usableH < 180 {
		usableH = 180
	}
	if usableH > h {
		usableH = h - headerH - statusH
	}
	targetW := usableW
	targetH := usableH
	idealH := int(float64(targetW) * 9.0 / 16.0)
	if idealH <= targetH {
		targetH = idealH
	} else {
		targetW = int(float64(targetH) * 16.0 / 9.0)
	}
	if cp, ok := rv.CapturePrev.(*capturePreview); ok {
		cp.setTargetSize(targetW, targetH)
	}
}
