package view

import (
	"fmt"
	"image"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders
	. "modernc.org/tk9.0"
)

// SelectionOverlay manages the selection window that sets the capture rectangle.
type SelectionOverlay struct {
	logger    *slog.Logger
	cfg       *config.Config
	cfgPath   string
	selection atomic.Pointer[image.Rectangle]
	win       *ToplevelWidget
}

// NewSelectionOverlay creates a new overlay manager with the saved selection.
func NewSelectionOverlay(cfg *config.Config, cfgPath string, logger *slog.Logger) *SelectionOverlay {
	v := &SelectionOverlay{logger: logger, cfg: cfg, cfgPath: cfgPath}
	if cfg.SelectionW > 0 && cfg.SelectionH > 0 {
		rect := image.Rect(cfg.SelectionX, cfg.SelectionY, cfg.SelectionX+cfg.SelectionW, cfg.SelectionY+cfg.SelectionH)
		v.selection.Store(&rect)
	}
	return v
}

// OpenOrFocus opens the selection window unless it is already open.
func (v *SelectionOverlay) OpenOrFocus() {
	if v.win != nil {
		WmGeometry(v.win.Window)
		return
	}
	win := App.Toplevel(Borderwidth(2), Background("#008080"))
	win.WmTitle("Selection Grid")
	v.win = win
	screen := screenRect(v.cfg)
	screenW, screenH := screen.Dx(), screen.Dy()
	initW, initH := max(screenW*2/3, 1), max(screenH*5/9, 1)
	x, y := screen.Min.X+(screenW-initW)/2, screen.Min.Y+(screenH-initH)/2
	WmGeometry(win.Window, fmt.Sprintf("%dx%d+%d+%d", initW, initH, x, y))
	WmAttributes(win.Window, "-topmost", 1)
	WmAttributes(win.Window, "-toolwindow", true)
	WmAttributes(win.Window, "-transparentcolor", "#008080")
	GridRowConfigure(win.Window, 0, Weight(1))
	GridColumnConfigure(win.Window, 0, Weight(0))
	GridColumnConfigure(win.Window, 1, Weight(1))
	GridColumnConfigure(win.Window, 2, Weight(0))
	left := win.Frame(Width(4), Background("#FFFFFF"))
	Grid(left, Row(0), Column(0), Sticky("ns"))
	center := win.Frame(Background("#008080"))
	Grid(center, Row(0), Column(1), Sticky("nsew"))
	right := win.Frame(Width(4), Background("#FFFFFF"))
	Grid(right, Row(0), Column(2), Sticky("ns"))
	controls := win.Frame()
	Grid(controls, Row(1), Column(0), Columnspan(3), Sticky("we"))
	confirm := win.Button(Txt("Confirm [Enter]"), Command(v.confirm))
	Grid(confirm, In(controls), Row(0), Column(0), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	cancel := win.Button(Txt("Cancel [Esc]"), Command(v.destroy))
	Grid(cancel, In(controls), Row(0), Column(1), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	clear := win.Button(Txt("Clear"), Command(v.clear))
	Grid(clear, In(controls), Row(0), Column(2), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	Bind(win, "<Return>", Command(v.confirm))
	Bind(win, "<Escape>", Command(v.destroy))
}

func (v *SelectionOverlay) clear() {
	v.selection.Store(nil)
	v.cfg.SelectionW, v.cfg.SelectionH = 0, 0
	v.save()
}

func (v *SelectionOverlay) confirm() {
	if v.win == nil {
		return
	}
	if rect, ok := parseGeometry(WmGeometry(v.win.Window)); ok {
		v.selection.Store(&rect)
		v.cfg.SelectionX, v.cfg.SelectionY = rect.Min.X, rect.Min.Y
		v.cfg.SelectionW, v.cfg.SelectionH = rect.Dx(), rect.Dy()
		v.save()
	}
	v.destroy()
}

func (v *SelectionOverlay) save() {
	if err := v.cfg.Save(v.cfgPath); err != nil {
		v.logger.Error("save selection failed", "error", err)
	}
}

func (v *SelectionOverlay) destroy() {
	if v.win != nil {
		Destroy(v.win)
		v.win = nil
	}
}

// ActiveRect returns the selected rectangle, or nil. Safe from any goroutine.
func (v *SelectionOverlay) ActiveRect() *image.Rectangle { return v.selection.Load() }

// screenRect returns the monitor the overlay should open on: the configured
// screen if it exists, otherwise a default 1920x1080 area at the origin.
func screenRect(cfg *config.Config) image.Rectangle {
	if b, ok := action.ScreenBounds(cfg.Screen); ok {
		return b
	}
	return image.Rect(0, 0, 1920, 1080)
}

// geometryRe matches window geometry strings in the format "WIDTHxHEIGHT+X+Y".
var geometryRe = regexp.MustCompile(`^(\d+)x(\d+)[+-](-?\d+)[+-](-?\d+)$`)

// parseGeometry parses a Tk geometry string into a rectangle.
func parseGeometry(g string) (image.Rectangle, bool) {
	m := geometryRe.FindStringSubmatch(strings.TrimSpace(g))
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
