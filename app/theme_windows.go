package app

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// themeIcons are the theme icons, rendered from frontend/public/themes by
// scripts/theme-icons.ps1.
//
//go:embed icons/*.png
var themeIcons embed.FS

// setTheme shows the theme's icon on the main window and its taskbar button.
// The exe itself keeps the default theme's icon.
func (a *app) setTheme(theme string) {
	png, err := themeIcons.ReadFile("icons/" + theme + ".png")
	if err != nil {
		a.logger.Error("theme icon missing", "theme", theme, "error", err)
		return
	}
	big, errBig := w32.CreateLargeHIconFromImage(png)
	small, errSmall := w32.CreateSmallHIconFromImage(png)
	if errBig != nil || errSmall != nil {
		a.logger.Error("theme icon failed", "theme", theme, "big", errBig, "small", errSmall)
		return
	}
	application.InvokeAsync(func() {
		hwnd := w32.HWND(uintptr(a.main.NativeWindow()))
		if hwnd == 0 {
			return
		}
		// WM_SETICON returns the icon it replaces; the window owns ours from now on.
		for size, icon := range map[uintptr]w32.HICON{w32.ICON_BIG: big, w32.ICON_SMALL: small} {
			if old := w32.SendMessage(hwnd, w32.WM_SETICON, size, uintptr(icon)); old != 0 {
				w32.DestroyIcon(w32.HICON(old))
			}
		}
	})
}
