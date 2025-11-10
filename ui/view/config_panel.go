package view

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/soocke/pixel-bot-go/config"

	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// ConfigPanel encapsulates the configuration form widgets and apply logic.
// It owns its widgets and writes back into *config.Config on ApplyChanges.
type ConfigPanel interface {
	Build(startRow int) (endRow int) // constructs widgets starting at startRow, returns next free row
	SetEditable(enabled bool)
	ApplyChanges() // parses widget text into underlying config and persists
}

type configPanel struct {
	cfg      *config.Config
	cfgPath  string
	logger   *slog.Logger
	applyBtn *ButtonWidget
	widgets  map[string]*TextWidget // keyed by internal field id
}

// NewConfigPanel creates the view bound to cfg.
func NewConfigPanel(cfg *config.Config, cfgPath string, logger *slog.Logger) ConfigPanel {
	return &configPanel{cfg: cfg, cfgPath: cfgPath, logger: logger, widgets: make(map[string]*TextWidget)}
}

func (v *configPanel) Build(startRow int) (row int) {
	c := v.cfg
	row = startRow
	makeRow := func(id, label, value string) {
		lbl := Label(Txt(label), Anchor("w"))
		Grid(lbl, Row(row), Column(0), Sticky("w"), Padx("0.4m"), Pady("0.15m"))
		w := Text(Height(1), Width(16))
		Grid(w, Row(row), Column(1), Sticky("we"), Padx("0.4m"), Pady("0.15m"))
		w.Delete("1.0", END)
		w.Insert("1.0", value)
		v.widgets[id] = w
		row++
	}
	makeRow("minScale", "Min Scale", fmt.Sprintf("%.2f", c.MinScale))
	makeRow("maxScale", "Max Scale", fmt.Sprintf("%.2f", c.MaxScale))
	makeRow("scaleStep", "Scale Step", fmt.Sprintf("%.3f", c.ScaleStep))
	makeRow("threshold", "Threshold", fmt.Sprintf("%.3f", c.Threshold))
	makeRow("stride", "Stride", fmt.Sprintf("%d", c.Stride))
	makeRow("stopOnScore", "Stop On Score", fmt.Sprintf("%.3f", c.StopOnScore))
	makeRow("refine", "Refine (true/false)", fmt.Sprintf("%t", c.Refine))
	makeRow("returnBestEven", "Return Best Even (true/false)", fmt.Sprintf("%t", c.ReturnBestEven))
	makeRow("reelKey", "Reel Key (e.g. F3 or R)", c.ReelKey)
	makeRow("roiSizePx", "ROI Size Px", fmt.Sprintf("%d", c.ROISizePx))
	makeRow("cooldownSeconds", "Cooldown Seconds", fmt.Sprintf("%d", c.CooldownSeconds))
	makeRow("maxCastDurationSeconds", "Max Cast Duration Seconds", fmt.Sprintf("%d", c.MaxCastDurationSeconds))
	makeRow("analysisScale", "Analysis Scale (0.2-1.0)", fmt.Sprintf("%.2f", c.AnalysisScale))
	v.applyBtn = Button(Txt("Apply Changes"), Command(func() { v.ApplyChanges() }))
	Grid(v.applyBtn, Row(row), Column(0), Columnspan(2), Sticky("we"), Padx("0.4m"), Pady("0.3m"))
	row++
	return row
}

func (v *configPanel) SetEditable(enabled bool) {
	state := "disabled"
	if enabled {
		state = "normal"
	}
	for _, w := range v.widgets {
		if w != nil {
			w.Configure(State(state))
		}
	}
	if v.applyBtn != nil {
		v.applyBtn.Configure(State(state))
	}
}

func (v *configPanel) text(w *TextWidget) string {
	if w == nil {
		return ""
	}
	parts := w.Get("1.0", END)
	return strings.Join(parts, "")
}

func (v *configPanel) ApplyChanges() {
	if v.cfg == nil {
		return
	}
	cfg := *v.cfg // copy
	assignFloat := func(id string, dst *float64) {
		w := v.widgets[id]
		if w == nil {
			return
		}
		if f, ok := parseFloatField(strings.TrimSpace(v.text(w))); ok {
			*dst = f
		}
	}
	assignInt := func(id string, dst *int) {
		w := v.widgets[id]
		if w == nil {
			return
		}
		if i, ok := parseIntField(strings.TrimSpace(v.text(w))); ok {
			*dst = i
		}
	}
	assignBool := func(id string, dst *bool) {
		w := v.widgets[id]
		if w == nil {
			return
		}
		if b, ok := parseBoolLoose(strings.TrimSpace(v.text(w))); ok {
			*dst = b
		}
	}
	assignFloat("minScale", &cfg.MinScale)
	assignFloat("maxScale", &cfg.MaxScale)
	assignFloat("scaleStep", &cfg.ScaleStep)
	assignFloat("threshold", &cfg.Threshold)
	assignInt("stride", &cfg.Stride)
	assignFloat("stopOnScore", &cfg.StopOnScore)
	assignBool("refine", &cfg.Refine)
	assignBool("returnBestEven", &cfg.ReturnBestEven)
	assignInt("cooldownSeconds", &cfg.CooldownSeconds)
	assignInt("maxCastDurationSeconds", &cfg.MaxCastDurationSeconds)
	assignFloat("analysisScale", &cfg.AnalysisScale)
	if w := v.widgets["reelKey"]; w != nil {
		val := strings.TrimSpace(v.text(w))
		if val != "" {
			cfg.ReelKey = val
		}
	}
	if verr := cfg.Validate(); verr != nil {
		return
	}
	*v.cfg = cfg
	if err := v.cfg.Save(v.cfgPath); err != nil {
		if v.logger != nil {
			v.logger.Error("config save failed", "error", err)
		}
	} else {
		if v.logger != nil {
			v.logger.Info("config saved", "path", v.cfgPath)
		}
	}
}

// parsing helpers (unexported)
func parseFloatField(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
func parseIntField(s string) (int, bool) {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return i, true
}
func parseBoolLoose(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "y", "on", "t":
		return true, true
	case "false", "0", "no", "n", "off", "f":
		return false, true
	default:
		return false, false
	}
}
