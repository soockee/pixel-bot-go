package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/soocke/pixel-bot-go/internal/update"
)

// updateEvery is how often a release build asks GitHub for a newer release;
// unauthenticated API calls are limited to 60 an hour per address.
const updateEvery = time.Hour

// checkUpdate removes the build a previous update replaced and looks for a
// newer release now and every updateEvery. Local builds, whose version is not
// a release tag, skip it.
func (a *app) checkUpdate() {
	exe, err := update.Executable()
	if err != nil {
		a.logger.Warn("update check skipped", "error", err)
		return
	}
	if err := update.Cleanup(exe); err != nil {
		a.logger.Warn("old build not removed", "error", err)
	}
	if !update.IsRelease(update.Version) {
		a.logger.Info("update check skipped: not a release build", "version", update.Version)
		a.container.View.SetUpdateCheck("", time.Time{}, time.Time{}, errLocalBuild.Error())
		return
	}
	go func() {
		for {
			if err := a.findUpdate(); err != nil {
				a.logger.Warn("update check failed", "error", err)
			}
			time.Sleep(updateEvery)
		}
	}()
}

var errLocalBuild = errors.New("this is a local build; only release builds update")

// findUpdate asks GitHub for the latest release and offers it when it is
// newer than this build. It runs on the caller's goroutine.
func (a *app) findUpdate() error {
	if !update.IsRelease(update.Version) {
		return errLocalBuild
	}
	view := a.container.View
	busy := false
	a.loop.Do(func() {
		if busy = view.State().Checking; !busy {
			view.SetChecking()
		}
	})
	if busy {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := a.updates.Latest(ctx)
	a.loop.Do(func() {
		if err != nil {
			view.SetUpdateCheck(a.release.Tag, a.release.Published, time.Now(), err.Error())
			return
		}
		view.SetUpdateCheck(r.Tag, r.Published, time.Now(), "")
		st := view.State()
		if st.Updating {
			return // the release being installed stays
		}
		a.release = r
		if update.Newer(r.Tag, update.Version) && st.Update != r.Tag {
			a.logger.Info("update available", "version", update.Version, "latest", r.Tag)
			view.SetUpdate(r.Tag, false)
		}
	})
	return err
}

// releaseNotes opens the latest release's page in the browser.
func (a *app) releaseNotes() {
	if a.release.Tag == "" {
		return
	}
	a.wails.Browser.OpenURL(a.release.Notes())
}

// updateProblem says why an update cannot start now, or "" when it can. It
// runs on the loop.
func (a *app) updateProblem() string {
	st := a.container.View.State()
	switch {
	case !update.Newer(a.release.Tag, update.Version):
		return "no update is available"
	case a.container.Capture.Enabled():
		return "stop the run before updating"
	case st.Recording:
		return "stop the recording before updating"
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
	exe, err := update.Executable()
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
