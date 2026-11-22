package app

import (
	_ "embed"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// icon is the app icon, rendered from frontend/public/icon.svg by
// scripts/app-icon.ps1.
//
//go:embed icon.png
var icon []byte

// setIcon shows the app icon on the main window and its taskbar button.
func (a *app) setIcon() {
	big, errBig := w32.CreateLargeHIconFromImage(icon)
	small, errSmall := w32.CreateSmallHIconFromImage(icon)
	if errBig != nil || errSmall != nil {
		a.logger.Error("window icon failed", "big", errBig, "small", errSmall)
		return
	}
	application.InvokeAsync(func() {
		hwnd := w32.HWND(uintptr(a.main.NativeWindow()))
		if hwnd == 0 {
			return
		}
		// WM_SETICON returns the icon it replaces; the window owns ours from now on.
		for size, h := range map[uintptr]w32.HICON{w32.ICON_BIG: big, w32.ICON_SMALL: small} {
			if old := w32.SendMessage(hwnd, w32.WM_SETICON, size, uintptr(h)); old != 0 {
				w32.DestroyIcon(w32.HICON(old))
			}
		}
	})
}
