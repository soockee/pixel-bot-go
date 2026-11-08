package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/soocke/pixel-bot-go/internal/update"
)

// checkUpdate removes the build a previous update replaced and looks for a
// newer release. Local builds, whose version is not a release tag, skip it.
func (a *app) checkUpdate() {
	exe, err := os.Executable()
	if err != nil {
		a.logger.Warn("update check skipped", "error", err)
		return
	}
	if err := update.Cleanup(exe); err != nil {
		a.logger.Warn("old build not removed", "error", err)
	}
	if !update.IsRelease(update.Version) {
		a.logger.Info("update check skipped: not a release build", "version", update.Version)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		r, err := a.updates.Latest(ctx)
		if err != nil {
			a.logger.Warn("update check failed", "error", err)
			return
		}
		if !update.Newer(r.Tag, update.Version) {
			a.logger.Info("up to date", "version", update.Version, "latest", r.Tag)
			return
		}
		a.logger.Info("update available", "version", update.Version, "latest", r.Tag)
		a.loop.Do(func() {
			a.release = r
			a.container.View.SetUpdate(r.Tag, false)
		})
	}()
}

// updateProblem says why an update cannot start now, or "" when it can. It
// runs on the loop.
func (a *app) updateProblem() string {
	st := a.container.View.State()
	switch {
	case a.release.Tag == "":
		return "no update is available"
	case a.container.Capture.Enabled():
		return "stop the run before updating"
	case st.Recording:
		return "stop the recording before updating"
	case st.Uploading != "":
		return "wait for the upload to finish"
	}
	return ""
}

// installUpdate downloads and verifies the newer release, swaps it for the
// running executable and restarts. It runs on the caller's goroutine.
func (a *app) installUpdate() error {
	view := a.container.View
	var r update.Release
	var refusal error
	a.loop.Do(func() {
		if view.State().Updating {
			refusal = errors.New("an update is running")
		} else if p := a.updateProblem(); p != "" {
			refusal = errors.New(p)
		} else {
			r = a.release
			view.SetUpdate(r.Tag, true)
			view.SetStatus("Downloading " + r.Tag)
		}
	})
	if refusal != nil {
		return refusal
	}

	err := a.tryUpdate(r)
	a.loop.Do(func() {
		if err != nil {
			view.SetUpdate(r.Tag, false)
			view.SetStatus("Update failed: " + err.Error())
			return
		}
		view.SetStatus("Updated to " + r.Tag + ", restarting")
	})
	if err != nil {
		a.logger.Error("update failed", "release", r.Tag, "error", err)
		return err
	}
	a.logger.Info("update installed", "from", update.Version, "to", r.Tag)
	go a.wails.Quit() // Run starts the new build once the app has shut down
	return nil
}

func (a *app) tryUpdate(r update.Release) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	next := exe + ".new"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	shown := -1
	err = a.updates.Fetch(ctx, r, next, func(done, total int64) {
		if total <= 0 || int(done*100/total) == shown {
			return
		}
		shown = int(done * 100 / total)
		text := fmt.Sprintf("Downloading %s: %d%%", r.Tag, shown)
		a.loop.Do(func() { a.container.View.SetStatus(text) })
	})
	if err != nil {
		return err
	}
	// A run or recording may have started during the download.
	a.loop.Do(func() {
		if p := a.updateProblem(); p != "" {
			err = errors.New(p)
		}
	})
	if err == nil {
		err = update.Install(exe, next)
	}
	if err != nil {
		os.Remove(next)
		return err
	}
	a.restart = exe
	return nil
}
