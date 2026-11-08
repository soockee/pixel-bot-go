package main

const cycleToleranceUS = 250_000

type gameplayAudit struct {
	Cycles         int      `json:"complete_cast_cycles"`
	ReeledCycles   int      `json:"cycles_with_reel"`
	ScoredMotifs   int      `json:"scored_two_cycle_motifs"`
	RepeatedMotifs int      `json:"repeated_two_cycle_motifs"`
	RepeatFraction *float64 `json:"repeat_fraction,omitempty"`
}

type castCycle struct {
	duration, firstReel, lastReel int64
	reels                         int
}

func (c castCycle) near(other castCycle) bool {
	difference := func(a, b int64) int64 { return max(a, b) - min(a, b) }
	return c.reels == other.reels &&
		difference(c.duration, other.duration) <= cycleToleranceUS &&
		difference(c.firstReel, other.firstReel) <= cycleToleranceUS &&
		difference(c.duration-c.lastReel, other.duration-other.lastReel) <= cycleToleranceUS
}

type gameplayHistory struct {
	total   gameplayAudit
	history []castCycle
	current castCycle
	castUS  int64
	active  bool
}

func (h *gameplayHistory) cast(at int64) {
	if h.active && at > h.castUS {
		h.current.duration = at - h.castUS
		h.observe(h.current)
	}
	h.current = castCycle{}
	h.castUS, h.active = at, true
}

func (h *gameplayHistory) reel(at int64) {
	if !h.active || at < h.castUS {
		return
	}
	offset := at - h.castUS
	if h.current.reels == 0 {
		h.current.firstReel = offset
	}
	h.current.lastReel = offset
	h.current.reels++
}

func (h *gameplayHistory) gap() {
	h.active = false
	h.history = h.history[:0]
}

func (h *gameplayHistory) observe(c castCycle) {
	h.total.Cycles++
	if c.reels > 0 {
		h.total.ReeledCycles++
	}
	// The latest two cycles are compared only with earlier nonoverlapping pairs.
	const cycleHistoryLimit = 16
	if len(h.history) == cycleHistoryLimit {
		h.history = h.history[1:]
	}
	h.history = append(h.history, c)
	n := len(h.history)
	if n < 4 {
		return
	}
	h.total.ScoredMotifs++
	for i := 0; i < n-3; i++ {
		if h.history[i].near(h.history[n-2]) && h.history[i+1].near(h.history[n-1]) {
			h.total.RepeatedMotifs++
			break
		}
	}
}

func (a gameplayAudit) result() *gameplayAudit {
	if a.Cycles == 0 {
		return nil
	}
	if a.ScoredMotifs > 0 {
		fraction := float64(a.RepeatedMotifs) / float64(a.ScoredMotifs)
		a.RepeatFraction = &fraction
	}
	return &a
}

func auditReviewedGameplay(casts []timingCast) *gameplayAudit {
	var h gameplayHistory
	for i, c := range casts {
		if !c.nextCast || c.Review == nil || c.Review.Outcome == "uncertain" ||
			casts[i+1].Review == nil || casts[i+1].Review.Outcome == "uncertain" {
			h.gap()
			continue
		}
		cycle := castCycle{duration: c.EndUS - c.Cast.PressUS}
		if c.Review.ReelUS != nil {
			cycle.firstReel = *c.Review.ReelUS - c.Cast.PressUS
			cycle.lastReel, cycle.reels = cycle.firstReel, 1
		}
		h.observe(cycle)
	}
	return h.total.result()
}
