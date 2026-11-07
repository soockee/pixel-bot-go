package view

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/action"
	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/ui/theme"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// ConfigPanel is the configuration form and its apply logic.
type ConfigPanel struct {
	cfg      *config.Config
	cfgPath  string
	logger   *slog.Logger
	status   func(string)
	applyBtn *ButtonWidget
	widgets  map[string]*TextWidget
	labels   map[string]string
}

// NewConfigPanel creates a panel bound to cfg. status shows the result of Apply.
func NewConfigPanel(cfg *config.Config, cfgPath string, logger *slog.Logger, status func(string)) *ConfigPanel {
	return &ConfigPanel{cfg: cfg, cfgPath: cfgPath, logger: logger, status: status, widgets: map[string]*TextWidget{}, labels: map[string]string{}}
}

// Build creates the form inside target.
func (v *ConfigPanel) Build(target Widget) {
	c := v.cfg
	row := 0
	pal := theme.CurrentPalette()
	makeRow := func(id, label, value string) {
		lbl := Label(Txt(label), Anchor("w"), Background(pal.Surface), Foreground(pal.Text))
		Grid(lbl, In(target), Row(row), Column(0), Sticky("w"), Padx("0.4m"), Pady("0.15m"))
		w := Text(Height(1), Width(16), Background("white"), Foreground("black"))
		if theme.IsDark() {
			w.Configure(Background("#334155"), Foreground(pal.Text))
		}
		Grid(w, In(target), Row(row), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.15m"))
		w.Delete("1.0", END)
		w.Insert("1.0", value)
		v.widgets[id] = w
		v.labels[id] = label
		row++
	}
	makeRow("minScale", "Min Scale", fmt.Sprintf("%.2f", c.MinScale))
	makeRow("maxScale", "Max Scale", fmt.Sprintf("%.2f", c.MaxScale))
	makeRow("scaleStep", "Scale Step", fmt.Sprintf("%.3f", c.ScaleStep))
	makeRow("threshold", "Threshold", fmt.Sprintf("%.3f", c.Threshold))
	makeRow("stride", "Stride", fmt.Sprintf("%d", c.Stride))
	makeRow("refine", "Refine (true/false)", fmt.Sprintf("%t", c.Refine))
	makeRow("castKey", "Cast Key (e.g. F3 or R)", c.CastKey)
	makeRow("stopKey", "Stop Hotkey (after restart)", c.StopKey)
	makeRow("maxFailedCasts", "Max Failed Casts", fmt.Sprintf("%d", c.MaxFailedCasts))
	makeRow("maxSessionMinutes", "Max Session Minutes", fmt.Sprintf("%d", c.MaxSessionMinutes))
	makeRow("locator", "Locator ("+strings.Join(detect.LocatorNames(), ", ")+")", c.Locator)
	makeRow("biteDetector", "Bite Detector ("+strings.Join(detect.BiteDetectorNames(), ", ")+")", c.BiteDetector)
	makeRow("recordDir", "Recording Folder", c.RecordDir)
	makeRow("recordKeys", "Record Keys (true/false)", fmt.Sprintf("%t", c.RecordKeys))
	makeRow("roiSizePx", "ROI Size Px", fmt.Sprintf("%d", c.ROISizePx))
	makeRow("cooldownSeconds", "Cooldown Seconds", fmt.Sprintf("%d", c.CooldownSeconds))
	makeRow("maxCastDurationSeconds", "Max Cast Duration Seconds", fmt.Sprintf("%d", c.MaxCastDurationSeconds))
	v.applyBtn = Button(Txt("Apply Changes"), Background(pal.Primary), Foreground("white"), Relief("raised"), Borderwidth(1), Command(v.apply))
	Grid(v.applyBtn, In(target), Row(row), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
}

// SetEditable enables or disables the form.
func (v *ConfigPanel) SetEditable(enabled bool) {
	state := buttonState(enabled)
	for _, w := range v.widgets {
		w.Configure(State(state))
	}
	if v.applyBtn != nil {
		v.applyBtn.Configure(State(state))
	}
}

// apply saves the form, or names the invalid fields and changes nothing.
func (v *ConfigPanel) apply() {
	cfg := *v.cfg
	var invalid []string
	field := func(id string, parse func(string) bool) {
		if !parse(strings.TrimSpace(strings.Join(v.widgets[id].Get("1.0", END), ""))) {
			invalid = append(invalid, v.labels[id])
		}
	}
	float := func(id string, dst *float64) {
		field(id, func(s string) bool { return parseInto(s, dst, parseFloat) })
	}
	integer := func(id string, dst *int) {
		field(id, func(s string) bool { return parseInto(s, dst, strconv.Atoi) })
	}
	boolean := func(id string, dst *bool) {
		field(id, func(s string) bool { return parseInto(s, dst, parseBool) })
	}
	key := func(id string, dst *string) {
		field(id, func(s string) bool {
			_, ok := action.ParseVK(s)
			if ok {
				*dst = s
			}
			return ok
		})
	}
	float("minScale", &cfg.MinScale)
	float("maxScale", &cfg.MaxScale)
	float("scaleStep", &cfg.ScaleStep)
	float("threshold", &cfg.Threshold)
	integer("stride", &cfg.Stride)
	boolean("refine", &cfg.Refine)
	key("castKey", &cfg.CastKey)
	key("stopKey", &cfg.StopKey)
	integer("maxFailedCasts", &cfg.MaxFailedCasts)
	integer("maxSessionMinutes", &cfg.MaxSessionMinutes)
	choice := func(id string, dst *string, names []string) {
		field(id, func(s string) bool {
			*dst = s
			return slices.Contains(names, s)
		})
	}
	choice("locator", &cfg.Locator, detect.LocatorNames())
	choice("biteDetector", &cfg.BiteDetector, detect.BiteDetectorNames())
	field("recordDir", func(s string) bool {
		cfg.RecordDir = s
		return s != ""
	})
	boolean("recordKeys", &cfg.RecordKeys)
	integer("roiSizePx", &cfg.ROISizePx)
	integer("cooldownSeconds", &cfg.CooldownSeconds)
	integer("maxCastDurationSeconds", &cfg.MaxCastDurationSeconds)
	if len(invalid) > 0 {
		v.status("Not saved, invalid: " + strings.Join(invalid, ", "))
		return
	}
	*v.cfg = cfg
	if err := v.cfg.Save(v.cfgPath); err != nil {
		v.logger.Error("config save failed", "error", err)
		v.status("Config not saved: " + err.Error())
		return
	}
	v.logger.Info("config saved", "path", v.cfgPath)
	v.status("Config saved")
}

func parseInto[T any](s string, dst *T, parse func(string) (T, error)) bool {
	value, err := parse(s)
	if err != nil {
		return false
	}
	*dst = value
	return true
}

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "y", "on", "t":
		return true, nil
	case "false", "0", "no", "n", "off", "f":
		return false, nil
	}
	return false, strconv.ErrSyntax
}
