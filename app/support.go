package app

import (
	"time"

	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

// countCatches adds the run's new catches to the total on this computer and,
// when a run ends, saves the total and decides on the thank-you card. A dry
// run's catches do not count. It runs on the loop.
func (a *app) countCatches(reels int, running bool) {
	c := a.container
	cfg := c.Config
	if cfg.DryRun {
		return
	}
	if reels > a.reelsSeen {
		cfg.CatchesTotal += reels - a.reelsSeen
	}
	a.reelsSeen = reels // a new run counts from 0 again
	ended := a.wasRunning && !running
	a.wasRunning = running
	if !ended {
		return
	}
	now := time.Now()
	if desktop.AskForSupport(reels, cfg.CatchesTotal, unixTime(cfg.SupportAskedAt), unixTime(cfg.SupportOpenedAt), now, cfg.DryRun) {
		cfg.SupportAskedAt = now.Unix()
		c.View.SetSupport(desktop.Support{Ask: true, RunCatches: reels, Catches: cfg.CatchesTotal})
	}
	if err := cfg.Save(config.FilePath); err != nil {
		a.logger.Error("catch count not saved", "error", err)
	}
}

// unixTime turns Unix seconds into a time, 0 into the zero time.
func unixTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
