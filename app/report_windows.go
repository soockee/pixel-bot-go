package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

// reportProblem opens a new GitHub issue, filled in with what the app knows,
// in the browser. It runs on the loop.
func (a *app) reportProblem() {
	c := a.container
	url := desktop.ProblemURL(c.View.State(), c.Config, windowsVersion(), desktop.RunSummary(config.LogPath), desktop.RecentProblems(config.LogPath, 10))
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

// saveReport writes a report file next to the logs, for a bug report where
// the log was asked for, and shows it selected in File Explorer, ready to drag
// into the issue. It returns the file's path. It runs on the loop.
func (a *app) saveReport() (string, error) {
	c := a.container
	details := fmt.Sprintf("Go Fish %s on %s\n\n%s", c.View.State().Version, windowsVersion(),
		desktop.ProblemDetails(c.View.State(), c.Config, desktop.RunSummary(config.LogPath), desktop.RecentProblems(config.LogPath, 10)))
	path, err := filepath.Abs("go-fish-report-" + time.Now().Format("20060102-150405") + ".zip")
	if err != nil {
		return "", err
	}
	if err := writeReport(path, details); err != nil {
		return "", err
	}
	// explorer.exe takes /select, with the path quoted inside, which Go's own quoting would break.
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	if err := cmd.Start(); err != nil {
		a.logger.Warn("could not show the report file", "error", err)
	} else {
		go cmd.Wait() // explorer.exe returns at once; release its process handle
	}
	return path, nil
}
