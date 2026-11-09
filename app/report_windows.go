package app

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

// reportProblem opens a new GitHub issue, filled in with what the app knows,
// in the browser. It runs on the loop.
func (a *app) reportProblem() {
	c := a.container
	url := desktop.ProblemURL(c.View.State(), c.Config, windowsVersion(), desktop.RecentProblems(config.LogPath, 10))
	a.wails.Browser.OpenURL(url)
}

// windowsVersion names the running Windows, for example "Windows 11 (build 26100)".
func windowsVersion() string {
	v := windows.RtlGetVersion()
	name := fmt.Sprintf("Windows %d.%d", v.MajorVersion, v.MinorVersion)
	if v.MajorVersion == 10 {
		name = "Windows 10"
		if v.BuildNumber >= 22000 { // Windows 11 still reports 10.0
			name = "Windows 11"
		}
	}
	return fmt.Sprintf("%s (build %d)", name, v.BuildNumber)
}
