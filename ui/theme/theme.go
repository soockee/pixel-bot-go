package theme

// Centralized theming and styling initialization for the pixel bot UI.
// Provides palette constants and InitStyles to activate a base theme and
// configure semantic widget styles.

import (
	//lint:ignore ST1001 Dot import is intentional for concise Tk widget DSL builders.
	. "modernc.org/tk9.0"
)

// Palette defines core semantic colors used across widgets.
// These can later be switched dynamically (e.g., dark mode) by re-calling InitStylesDark.
const (
	ColorBg        = "#f7f9fb" // app background
	ColorSurface   = "#ffffff" // panels, cards
	ColorBorder    = "#d0d7de"
	ColorPrimary   = "#2563eb" // buttons, accents
	ColorPrimaryHi = "#1d4ed8"
	ColorDanger    = "#dc2626"
	ColorDangerHi  = "#b91c1c"
	ColorAccent    = "#10b981"
	ColorText      = "#1e293b"
	ColorTextMuted = "#64748b"
)

// PaletteSnapshot represents resolved colors for the active mode.
type PaletteSnapshot struct {
	AppBg     string
	Surface   string
	Border    string
	Primary   string
	Danger    string
	Accent    string
	Text      string
	TextMuted string
}

// CurrentPalette returns colors for the current dark/light mode.
func CurrentPalette() PaletteSnapshot {
	if darkMode {
		return PaletteSnapshot{
			AppBg:     "#0f172a",
			Surface:   "#1e293b",
			Border:    "#334155",
			Primary:   "#3b82f6",
			Danger:    "#ef4444",
			Accent:    "#10b981",
			Text:      "#f1f5f9",
			TextMuted: "#94a3b8",
		}
	}
	return PaletteSnapshot{
		AppBg:     ColorBg,
		Surface:   ColorSurface,
		Border:    ColorBorder,
		Primary:   ColorPrimary,
		Danger:    ColorDanger,
		Accent:    ColorAccent,
		Text:      ColorText,
		TextMuted: ColorTextMuted,
	}
}

// style names used with Style("primary.TButton") etc.
const (
	StylePrimaryButton = "primary.TButton"
	StyleDangerButton  = "danger.TButton"
	StyleAccentLabel   = "accent.TLabel"
	StyleStateLabel    = "state.TLabel"
)

// internal flag for current mode
var darkMode bool

// InitStyles (re)applies styles for the current darkMode value.
func InitStyles() { applyStyles(darkMode) }

// SetDark toggles dark mode and reapplies styles. Returns new mode value.
func SetDark(dark bool) bool {
	darkMode = dark
	applyStyles(darkMode)
	return darkMode
}

// ToggleDark flips dark mode and reapplies styles. Returns new mode value.
func ToggleDark() bool { return SetDark(!darkMode) }

// IsDark reports current mode.
func IsDark() bool { return darkMode }

// applyStyles encapsulates palette & style configuration for light/dark.
func applyStyles(dark bool) {
	_ = ActivateTheme("azure light") // baseline metrics
	if dark {
		App.Configure(Background("#0f172a"))
	} else {
		App.Configure(Background(ColorBg))
	}

	// Primary button
	StyleConfigure(StylePrimaryButton,
		Background(func() string {
			if dark {
				return "#3b82f6"
			}
			return ColorPrimary
		}()),
		Foreground("white"),
		Padding("4p 3p"),
		Borderwidth(1),
		Relief("ridge"),
	)
	// Danger button
	StyleConfigure(StyleDangerButton,
		Background(func() string {
			if dark {
				return "#ef4444"
			}
			return ColorDanger
		}()),
		Foreground("white"),
		Padding("4p 3p"),
		Borderwidth(1),
		Relief("ridge"),
	)
	// Accent label
	StyleConfigure(StyleAccentLabel,
		Foreground(func() string {
			if dark {
				return "#3b82f6"
			}
			return ColorPrimary
		}()),
		Background(func() string {
			if dark {
				return "#1e293b"
			}
			return ColorSurface
		}()),
		Padding("2p 1p"),
	)
	// State label
	StyleConfigure(StyleStateLabel,
		Foreground(func() string {
			if dark {
				return "#f0fdf4"
			}
			return "white"
		}()),
		Background(func() string {
			if dark {
				return "#10b981"
			}
			return ColorAccent
		}()),
		Padding("4p 2p"),
		Borderwidth(1),
		Relief("groove"),
	)
}
