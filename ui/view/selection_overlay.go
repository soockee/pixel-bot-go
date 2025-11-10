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

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders
	. "modernc.org/tk9.0"
)

// SelectionOverlay manages the optional selection window allowing the user
// to constrain screen capture to a rectangle.
type SelectionOverlay interface {
	OpenOrFocus()
	Clear()
	ActiveRect() *image.Rectangle
}

type selectionOverlay struct {
	logger    *slog.Logger
	cfg       *config.Config
	cfgPath   string
	selection atomic.Value // stores image.Rectangle
	win       *ToplevelWidget
}

// NewSelectionOverlay creates a new overlay manager.
func NewSelectionOverlay(cfg *config.Config, cfgPath string, logger *slog.Logger) SelectionOverlay {
	v := &selectionOverlay{logger: logger, cfg: cfg, cfgPath: cfgPath}
	if cfg != nil && cfg.SelectionW > 0 && cfg.SelectionH > 0 {
		rect := image.Rect(cfg.SelectionX, cfg.SelectionY, cfg.SelectionX+cfg.SelectionW, cfg.SelectionY+cfg.SelectionH)
		v.selection.Store(rect)
	}
	return v
}

func (v *selectionOverlay) OpenOrFocus() {
	if v.win != nil {
		WmGeometry(v.win.Window)
		return
	}
	win := App.Toplevel(Borderwidth(2), Background("#008080"))
	win.WmTitle("Selection Grid")
	v.win = win
	cx, cy := computeCenteredGeometry()
	screenW, screenH := int(cx), int(cy)
	initW, initH := screenW*2/3, screenH*5/9
	if initW < 1 {
		initW = 1
	}
	if initH < 1 {
		initH = 1
	}
	x, y := (screenW-initW)/2, (screenH-initH)/2
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
	cancel := win.Button(Txt("Cancel [Esc]"), Command(v.cancel))
	Grid(cancel, In(controls), Row(0), Column(1), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	clear := win.Button(Txt("Clear"), Command(v.Clear))
	Grid(clear, In(controls), Row(0), Column(2), Sticky("we"), Padx("0.2m"), Pady("0.2m"))
	Bind(win, "<Return>", Command(v.confirm))
	Bind(win, "<Escape>", Command(v.cancel))
}

func (v *selectionOverlay) Clear() {
	v.selection.Store(image.Rectangle{})
	if v.cfg != nil {
		v.cfg.SelectionW, v.cfg.SelectionH = 0, 0
		_ = v.cfg.Save(v.cfgPath)
	}
}

func (v *selectionOverlay) confirm() {
	if v.win == nil {
		return
	}
	geom := WmGeometry(v.win.Window)
	if rect, ok := parseGeometrySel(geom); ok {
		v.selection.Store(rect)
		if v.cfg != nil {
			v.cfg.SelectionX, v.cfg.SelectionY = rect.Min.X, rect.Min.Y
			v.cfg.SelectionW, v.cfg.SelectionH = rect.Dx(), rect.Dy()
			_ = v.cfg.Save(v.cfgPath)
		}
	}
	v.destroy()
}

func (v *selectionOverlay) cancel() { v.destroy() }

func (v *selectionOverlay) destroy() {
	if v.win != nil {
		Destroy(v.win)
		v.win = nil
	}
}

func (v *selectionOverlay) ActiveRect() *image.Rectangle {
	rv := v.selection.Load()
	if rv == nil {
		return nil
	}
	r, ok := rv.(image.Rectangle)
	if !ok || r == (image.Rectangle{}) {
		return nil
	}
	return &r
}

// computeCenteredGeometry returns the screen width and height.
// Currently returns static values; should be replaced with proper Tk winfo queries.
func computeCenteredGeometry() (float64, float64) {
	return 1920, 1080
}

// geomReSel matches window geometry strings in the format "WIDTHxHEIGHT+X+Y"
var geomReSel = regexp.MustCompile(`^(\d+)x(\d+)\+(-?\d+)\+(-?\d+)$`)

// parseGeometrySel parses a Tk geometry string and returns the corresponding rectangle.
func parseGeometrySel(g string) (image.Rectangle, bool) {
	g = strings.TrimSpace(g)
	m := geomReSel.FindStringSubmatch(g)
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
