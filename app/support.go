package app

import (
	"github.com/soocke/pixel-bot-go/config"
	"github.com/soocke/pixel-bot-go/domain/fishing"
	"github.com/soocke/pixel-bot-go/ui/desktop"
)

// countCatches adds a run's catches and fishing time to the totals on this
// computer when it ends, saves them, and decides on the thank-you card. With
// Bobber Marker's outcomes the catches are the fish that came out; an older
// addon reports none, and then the reels count. A dry run counts nothing. It
// runs on the loop with the run's stats and the window's state of this tick.
func (a *app) countCatches(run fishing.Stats, s desktop.State) {
	cfg := a.container.Config
	if cfg.DryRun {
		return
	}
	if s.Running {
		a.wasRunning, a.runSeconds = true, s.SessionSeconds
		return
	}
	if !a.wasRunning {
		return
	}
	a.wasRunning = false
	catches := run.Caught
	if run.Caught+run.GotAway+run.NotHooked == 0 {
		catches = run.Reels
	}
	cfg.CatchesTotal += catches
	cfg.FishingSecondsTotal += a.runSeconds
	result := desktop.SupportRun{Catches: catches, NotHooked: run.NotHooked}
	if m, ok := desktop.SupportMilestone(cfg.CatchesTotal, cfg.SupportMilestone, result, cfg.SupportMuted, cfg.DryRun); ok {
		cfg.SupportMilestone = m
		a.container.View.SetSupport(desktop.Support{Ask: true, Milestone: m, Catches: cfg.CatchesTotal, Seconds: cfg.FishingSecondsTotal})
	}
	if err := cfg.Save(config.FilePath); err != nil {
		a.logger.Error("catch count not saved", "error", err)
	}
}
