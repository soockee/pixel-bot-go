package view

import (
	"image"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/ui/theme"

	//lint:ignore ST1001 Dot import for concise Tk widget DSL.
	. "modernc.org/tk9.0"
)

// RootCallbacks connects the root view to the rest of the app.
type RootCallbacks struct {
	ListWindows   func() []string
	Window        func() string // the selected window title
	SetWindow     func(title string)
	ToggleCapture func()
	OpenSelection func()
	Exit          func()
}

// RootView composes the top-level application layout and wires UI callbacks.
type RootView struct {
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger
	calls   RootCallbacks

	session          *SessionStats
	configPanel      *ConfigPanel
	preview          *CapturePreview
	stateLabel       *TLabelWidget
	windowSelect     *TComboboxWidget
	windowTitles     []string
	windowPicked     bool // the user or a flag chose the window
	statusLabel      *LabelWidget
	windowExplainLbl *TLabelWidget
	captureLabel     *LabelWidget
	detectionLabel   *LabelWidget
	refreshBtn       *ButtonWidget
	captureBtn       *ButtonWidget
	selectionBtn     *ButtonWidget
	exitBtn          *ButtonWidget
	configFrame      *FrameWidget
	mainFrame        *FrameWidget
	headerFrame      *FrameWidget
	leftInlineFrame  *FrameWidget
	actionsFrame     *FrameWidget
	statusBarFrame   *FrameWidget
	toggleConfigBtn  *ButtonWidget
	darkToggleBtn    *ButtonWidget
	configVisible    bool
	darkMode         bool
	running          bool
}

// NewRootView returns an unbuilt RootView.
func NewRootView(cfg *config.Config, cfgPath string, logger *slog.Logger) *RootView {
	return &RootView{cfg: cfg, cfgPath: cfgPath, logger: logger}
}

func (rv *RootView) newConfigPanel() *ConfigPanel {
	return NewConfigPanel(rv.cfg, rv.cfgPath, rv.logger, rv.SetStatus)
}

// primaryButton creates a button in the primary color.
func primaryButton(label string, pal theme.PaletteSnapshot, command func()) *ButtonWidget {
	return Button(Txt(label), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1), Command(command))
}

// Build constructs the layout.
func (rv *RootView) Build(calls RootCallbacks) {
	rv.calls = calls
	rv.windowPicked = calls.Window() != ""
	theme.InitStyles()
	if rv.cfg.DarkMode {
		theme.SetDark(true)
		rv.darkMode = true
	}

	GridRowConfigure(App, 0, Weight(0))
	GridRowConfigure(App, 1, Weight(1))
	GridRowConfigure(App, 2, Weight(0))
	GridColumnConfigure(App, 0, Weight(0))
	GridColumnConfigure(App, 1, Weight(1))

	pal := theme.CurrentPalette()

	rv.headerFrame = Frame(Background(pal.Surface), Borderwidth(0))
	Grid(rv.headerFrame, Row(0), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	GridColumnConfigure(rv.headerFrame, 0, Weight(0))
	GridColumnConfigure(rv.headerFrame, 1, Weight(1))
	GridColumnConfigure(rv.headerFrame, 2, Weight(0))

	rv.leftInlineFrame = Frame(Background(pal.Surface), Borderwidth(0))
	Grid(rv.leftInlineFrame, In(rv.headerFrame), Row(0), Column(0), Sticky("nw"))
	GridColumnConfigure(rv.leftInlineFrame, 0, Weight(0))
	GridColumnConfigure(rv.leftInlineFrame, 1, Weight(0))
	GridColumnConfigure(rv.leftInlineFrame, 2, Weight(0))

	rv.session = NewSessionStats(rv.leftInlineFrame, 0, 1)

	rv.actionsFrame = Frame(Background(pal.Surface))
	Grid(rv.actionsFrame, In(rv.headerFrame), Row(0), Column(1), Sticky("we"))
	GridColumnConfigure(rv.actionsFrame, 1, Weight(1))

	rv.stateLabel = TLabel(Txt("State: <none>"))
	Grid(rv.stateLabel, In(rv.headerFrame), Row(0), Column(2), Sticky("e"), Padx("0.3m"))

	rv.windowExplainLbl = TLabel(Txt("Target Window:"))
	Grid(rv.windowExplainLbl, In(rv.actionsFrame), Row(0), Column(0), Sticky("w"), Padx("0.2m"), Pady("0.2m"))
	rv.windowSelect = TCombobox(Width(26))
	Grid(rv.windowSelect, In(rv.actionsFrame), Row(0), Column(1), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	Bind(rv.windowSelect, "<<ComboboxSelected>>", Command(rv.windowChosen))
	rv.refreshBtn = primaryButton("Refresh", pal, rv.refreshWindows)
	Grid(rv.refreshBtn, In(rv.actionsFrame), Row(0), Column(2), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.captureBtn = primaryButton("Start", pal, calls.ToggleCapture)
	Grid(rv.captureBtn, In(rv.actionsFrame), Row(0), Column(3), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.selectionBtn = primaryButton("Selection", pal, calls.OpenSelection)
	Grid(rv.selectionBtn, In(rv.actionsFrame), Row(0), Column(4), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.exitBtn = Button(Txt("Exit"), Background(pal.Danger), Foreground("white"), Relief("raised"), Borderwidth(1), Command(calls.Exit))
	Grid(rv.exitBtn, In(rv.actionsFrame), Row(0), Column(5), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	rv.refreshWindows()

	rv.mainFrame = Frame(Background(pal.Surface), Relief("flat"))
	Grid(rv.mainFrame, Row(1), Column(0), Columnspan(2), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
	GridRowConfigure(rv.mainFrame, 0, Weight(1))
	GridColumnConfigure(rv.mainFrame, 0, Weight(1))

	rv.configPanel = rv.newConfigPanel()

	rv.captureLabel = Label(Relief("sunken"), Borderwidth(1))
	Grid(rv.captureLabel, In(rv.mainFrame), Row(0), Column(0), Sticky("nsew"), Padx("0.3m"), Pady("0.3m"))
	rv.detectionLabel = Label(Relief("sunken"), Borderwidth(1))
	Grid(rv.detectionLabel, In(rv.mainFrame), Row(0), Column(1), Sticky("n"), Padx("0.3m"), Pady("0.3m"))
	rv.preview = newCapturePreview(rv.captureLabel, rv.detectionLabel, rv.cfg.ROISizePx)
	Bind(App, "<Configure>", Command(rv.updatePreviewScale))

	rv.statusBarFrame = Frame(Background(pal.Surface))
	Grid(rv.statusBarFrame, Row(2), Column(0), Columnspan(2), Sticky("we"))
	rv.statusLabel = Label(Txt("Ready"), Anchor("w"))
	Grid(rv.statusLabel, In(rv.statusBarFrame), Row(0), Column(0), Sticky("w"), Padx("0.4m"), Pady("0.2m"))

	rv.toggleConfigBtn = primaryButton("Show Config", pal, rv.toggleConfig)
	Grid(rv.toggleConfigBtn, In(rv.leftInlineFrame), Row(0), Column(0), Sticky("w"), Padx("0.2m"), Pady("0.1m"))
	rv.darkToggleBtn = primaryButton("Dark Mode", pal, rv.toggleDarkMode)
	Grid(rv.darkToggleBtn, In(rv.leftInlineFrame), Row(0), Column(3), Sticky("w"), Padx("0.2m"), Pady("0.1m"))

	rv.applyPalette()
}

// defaultWindow is selected whenever it is open, until the user picks another.
const defaultWindow = "World of Warcraft"

// pickWindow returns the window to select. A window the user picked stays
// selected even if it is not open yet.
func pickWindow(titles []string, current string, picked bool) string {
	switch {
	case picked:
		return current
	case slices.Contains(titles, defaultWindow):
		return defaultWindow
	}
	return titles[0]
}

// refreshWindows reloads the dropdown and keeps the selection in step with it.
func (rv *RootView) refreshWindows() {
	titles := rv.calls.ListWindows()
	if len(titles) == 0 {
		titles = []string{"<none>"}
	}
	current := pickWindow(titles, rv.calls.Window(), rv.windowPicked)
	if !slices.Contains(titles, current) {
		titles = append(titles, current)
	}
	rv.windowTitles = titles
	rv.windowSelect.Configure(Values(titles))
	rv.windowSelect.Current(slices.Index(titles, current))
	rv.calls.SetWindow(current)
}

func (rv *RootView) windowChosen() {
	idx, err := strconv.Atoi(rv.windowSelect.Current(nil))
	if err != nil || idx < 0 || idx >= len(rv.windowTitles) {
		rv.logger.Error("window selection parse error", "error", err)
		return
	}
	rv.windowPicked = true
	rv.calls.SetWindow(rv.windowTitles[idx])
}

// SetStateLabel updates the state label text.
func (rv *RootView) SetStateLabel(text string) { rv.stateLabel.Configure(Txt(text)) }

// SetStatus shows a message in the status bar.
func (rv *RootView) SetStatus(text string) { rv.statusLabel.Configure(Txt(text)) }

// SetRunning shows the capture state on the button and locks the config while running.
func (rv *RootView) SetRunning(running bool) {
	rv.running = running
	rv.configPanel.SetEditable(!running)
	if running {
		rv.captureBtn.Configure(Txt("Stop"))
	} else {
		rv.captureBtn.Configure(Txt("Start"))
	}
}

// UpdateCapture shows a captured frame.
func (rv *RootView) UpdateCapture(img *image.RGBA) { rv.preview.UpdateCapture(img) }

// UpdateDetection shows the region around the bobber.
func (rv *RootView) UpdateDetection(img *image.RGBA) { rv.preview.UpdateDetection(img) }

// PreviewReset clears the previews.
func (rv *RootView) PreviewReset() { rv.preview.Reset() }

// SetSession updates both session and total capture durations.
func (rv *RootView) SetSession(session, total time.Duration) { rv.session.Set(session, total) }

// toggleConfig collapses or expands the config panel, re-gridding mainFrame accordingly.
func (rv *RootView) toggleConfig() {
	if rv.configVisible {
		Destroy(rv.configFrame)
		rv.configFrame = nil
		rv.configPanel = rv.newConfigPanel() // the old one's widgets are gone
		Grid(rv.mainFrame, Row(1), Column(0), Columnspan(2), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
		rv.toggleConfigBtn.Configure(Txt("Show Config"))
	} else {
		rv.configFrame = Frame(Background(theme.ColorSurface), Relief("groove"), Borderwidth(1))
		Grid(rv.configFrame, Row(1), Column(0), Sticky("ns"), Padx("0.4m"), Pady("0.2m"))
		GridRowConfigure(rv.configFrame, 0, Weight(0))
		GridColumnConfigure(rv.configFrame, 0, Weight(1))
		Grid(rv.mainFrame, Row(1), Column(1), Columnspan(1), Sticky("nsew"), Padx("0.4m"), Pady("0.2m"))
		rv.configPanel = rv.newConfigPanel()
		rv.configPanel.Build(rv.configFrame)
		rv.configPanel.SetEditable(!rv.running)
		rv.toggleConfigBtn.Configure(Txt("Hide Config"))
	}
	rv.configVisible = !rv.configVisible
	rv.updatePreviewScale()
	rv.applyPalette()
}

// toggleDarkMode flips theme dark/light and persists the choice.
func (rv *RootView) toggleDarkMode() {
	rv.darkMode = theme.ToggleDark()
	rv.applyPalette()
	rv.cfg.DarkMode = rv.darkMode
	if err := rv.cfg.Save(rv.cfgPath); err != nil {
		rv.logger.Error("persist dark mode failed", "error", err)
		return
	}
	rv.logger.Info("dark mode preference saved", "dark", rv.darkMode)
}

// applyPalette updates widget colors based on the current palette snapshot.
func (rv *RootView) applyPalette() {
	pal := theme.CurrentPalette()
	App.Configure(Background(pal.AppBg))
	for _, f := range []*FrameWidget{rv.headerFrame, rv.leftInlineFrame, rv.actionsFrame, rv.mainFrame, rv.statusBarFrame, rv.configFrame} {
		if f != nil { // configFrame exists only while the panel is open
			f.Configure(Background(pal.Surface))
		}
	}
	rv.windowExplainLbl.Configure(Background(pal.Surface), Foreground(pal.TextMuted))
	rv.stateLabel.Configure(Background(pal.Accent), Foreground("white"))
	rv.statusLabel.Configure(Background(pal.Surface), Foreground(pal.TextMuted))
	rv.captureLabel.Configure(Background(pal.Surface))
	rv.detectionLabel.Configure(Background(pal.Surface))
	if theme.IsDark() {
		rv.darkToggleBtn.Configure(Txt("Light Mode"))
	} else {
		rv.darkToggleBtn.Configure(Txt("Dark Mode"))
	}
	for _, b := range []*ButtonWidget{rv.toggleConfigBtn, rv.darkToggleBtn, rv.refreshBtn, rv.captureBtn, rv.selectionBtn} {
		b.Configure(Background(pal.Primary), Foreground("white"))
	}
	rv.exitBtn.Configure(Background(pal.Danger), Foreground("white"))
	for _, w := range rv.configPanel.widgets {
		if theme.IsDark() {
			w.Configure(Background("#334155"), Foreground(pal.Text))
		} else {
			w.Configure(Background("white"), Foreground("black"))
		}
	}
	if rv.configPanel.applyBtn != nil {
		rv.configPanel.applyBtn.Configure(Background(pal.Primary), Foreground("white"))
	}
}

// updatePreviewScale recalculates capture preview target size using window geometry.
func (rv *RootView) updatePreviewScale() {
	rect, ok := parseGeometry(WmGeometry(App))
	// Tk may report 1x1 before the window is mapped.
	if !ok || rect.Dx() < 400 || rect.Dy() < 300 {
		return
	}
	w, h := rect.Dx(), rect.Dy()
	const margin, headerH, statusH, configW = 32, 64, 30, 280
	usableW := w - rv.cfg.ROISizePx - margin
	if rv.configVisible {
		usableW -= configW
	}
	targetW := max(usableW, 320)
	targetH := max(h-headerH-statusH-margin, 180)
	if idealH := targetW * 9 / 16; idealH <= targetH {
		targetH = idealH
	} else {
		targetW = targetH * 16 / 9
	}
	rv.preview.setTargetSize(targetW, targetH)
}
