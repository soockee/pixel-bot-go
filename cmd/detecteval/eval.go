package main

import (
	"image"
	"slices"
	"time"

	"github.com/soocke/pixel-bot-go/domain/detect"
	"github.com/soocke/pixel-bot-go/internal/dataset"
)

const (
	biteEarlyToleranceMS = 200  // labels are only accurate to a frame or two
	biteMatchWindowMS    = 1500 // a later trigger cannot still catch the fish
)

// Counts are the raw results of one or more datasets. They depend only on the
// frames, labels and detectors, so two runs give identical counts.
type Counts struct {
	Casts           int     `json:"casts"`
	Locks           int     `json:"locks"`
	LocksOnBobber   int     `json:"locks_on_bobber"`
	EmptyFrames     int     `json:"empty_frames"`
	FalseLocks      int     `json:"false_locks"`
	BiteCasts       int     `json:"bite_casts"`
	BitesCaught     int     `json:"bites_caught"`
	FalseBites      int     `json:"false_bites"`
	BiteLatenciesMS []int64 `json:"bite_latencies_ms"`
}

func (c *Counts) add(o Counts) {
	c.Casts += o.Casts
	c.Locks += o.Locks
	c.LocksOnBobber += o.LocksOnBobber
	c.EmptyFrames += o.EmptyFrames
	c.FalseLocks += o.FalseLocks
	c.BiteCasts += o.BiteCasts
	c.BitesCaught += o.BitesCaught
	c.FalseBites += o.FalseBites
	c.BiteLatenciesMS = append(c.BiteLatenciesMS, o.BiteLatenciesMS...)
}

// percentile95 uses the nearest-rank method.
func percentile95[T int64 | time.Duration](values []T) T {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(values))
	return sorted[(len(sorted)*95+99)/100-1]
}

// evaluator runs one locator and one bite detector over datasets.
type evaluator struct {
	locator  detect.Locator
	bite     detect.BiteDetector
	roiSize  int
	searches []time.Duration // kept apart from Counts, because timings differ per run
}

func (e *evaluator) run(d *dataset.Dataset) (Counts, error) {
	var c Counts
	for _, cast := range d.Labels.Casts {
		frames := d.FramesIn(cast.Span)
		c.Casts++
		if err := e.locate(frames, cast.Bobber.Image(), &c); err != nil {
			return c, err
		}
		if err := e.watch(frames, cast, &c); err != nil {
			return c, err
		}
	}
	for _, span := range d.Labels.Empty {
		for _, f := range d.FramesIn(span) {
			img, err := f.Load()
			if err != nil {
				return c, err
			}
			c.EmptyFrames++
			if _, found := e.search(img); found {
				c.FalseLocks++
			}
		}
	}
	return c, nil
}

func (e *evaluator) search(img *image.RGBA) (detect.Match, bool) {
	start := time.Now()
	match, found := e.locator.Locate(img)
	e.searches = append(e.searches, time.Since(start))
	return match, found
}

// locate stops at the first match, because that is what the app would lock on.
func (e *evaluator) locate(frames []dataset.FrameRef, bobber image.Rectangle, c *Counts) error {
	for _, f := range frames {
		img, err := f.Load()
		if err != nil {
			return err
		}
		if match, found := e.search(img); found {
			c.Locks++
			if image.Pt(match.X, match.Y).In(bobber) {
				c.LocksOnBobber++
			}
			return nil
		}
	}
	return nil
}

// watch runs the bite detector around the labelled bobber, so bite results do
// not depend on the locator.
func (e *evaluator) watch(frames []dataset.FrameRef, cast dataset.Cast, c *Counts) error {
	if cast.BiteMS != nil {
		c.BiteCasts++
	}
	centre := cast.Bobber.Image()
	cx, cy := (centre.Min.X+centre.Max.X)/2, (centre.Min.Y+centre.Max.Y)/2
	e.bite.Reset()
	for _, f := range frames {
		img, err := f.Load()
		if err != nil {
			return err
		}
		roi, _, err := detect.ExtractROI(img, cx, cy, e.roiSize)
		if err != nil {
			return err
		}
		if e.bite.Feed(roi, time.UnixMilli(f.MS)) {
			scoreBite(f.MS, cast.BiteMS, c)
			return nil
		}
	}
	return nil
}

func scoreBite(triggerMS int64, biteMS *int64, c *Counts) {
	switch {
	case biteMS == nil || triggerMS < *biteMS-biteEarlyToleranceMS:
		c.FalseBites++
	case triggerMS <= *biteMS+biteMatchWindowMS:
		c.BitesCaught++
		c.BiteLatenciesMS = append(c.BiteLatenciesMS, max(triggerMS-*biteMS, 0))
	}
}
